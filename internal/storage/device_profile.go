package storage

import (
	"bytes"
	"context"
	"encoding/gob"
	"time"

	"github.com/liuhw0/chirpstack-network-server/v3/internal/logging"
	"github.com/go-redis/redis/v8"
	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Templates used for generating Redis keys
const (
	DeviceProfileKeyTempl = "lora:ns:dp:%s"
)

// DeviceProfile defines the backend.DeviceProfile with some extra meta-data
type DeviceProfile struct {
	CreatedAt          time.Time `db:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"`
	ID                 uuid.UUID `db:"device_profile_id"`
	SupportsClassB     bool      `db:"supports_class_b"`
	ClassBTimeout      int       `db:"class_b_timeout"` // Unit: seconds
	PingSlotPeriod     int       `db:"ping_slot_period"`
	PingSlotDR         int       `db:"ping_slot_dr"`
	PingSlotFreq       uint32    `db:"ping_slot_freq"` // in Hz
	SupportsClassC     bool      `db:"supports_class_c"`
	ClassCTimeout      int       `db:"class_c_timeout"`     // Unit: seconds
	MACVersion         string    `db:"mac_version"`         // Example: "1.0.2" [LW102]
	RegParamsRevision  string    `db:"reg_params_revision"` // Example: "B" [RP102B]
	RXDelay1           int       `db:"rx_delay_1"`
	RXDROffset1        int       `db:"rx_dr_offset_1"`
	RXDataRate2        int       `db:"rx_data_rate_2"`       // Unit: bits-per-second
	RXFreq2            uint32    `db:"rx_freq_2"`            // In Hz
	FactoryPresetFreqs []uint32  `db:"factory_preset_freqs"` // In Hz
	MaxEIRP            int       `db:"max_eirp"`             // In dBm
	MaxDutyCycle       int       `db:"max_duty_cycle"`       // Example: 10 indicates 10%
	SupportsJoin       bool      `db:"supports_join"`
	RFRegion           string    `db:"rf_region"`
	Supports32bitFCnt  bool      `db:"supports_32bit_fcnt"`
	ADRAlgorithmID     string    `db:"adr_algorithm_id"`
}

// CreateDeviceProfile creates the given device-profile.
func CreateDeviceProfile(ctx context.Context, db sqlx.Execer, dp *DeviceProfile) error {
	now := time.Now()

	if dp.ID == uuid.Nil {
		var err error
		dp.ID, err = uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
	}

	dp.CreatedAt = now
	dp.UpdatedAt = now

	_, err := db.Exec(`
        insert into device_profile (
            created_at,
            updated_at,

            device_profile_id,
            supports_class_b,
            class_b_timeout,
            ping_slot_period,
            ping_slot_dr,
            ping_slot_freq,
            supports_class_c,
            class_c_timeout,
            mac_version,
            reg_params_revision,
            rx_delay_1,
            rx_dr_offset_1,
            rx_data_rate_2,
            rx_freq_2,
            factory_preset_freqs,
            max_eirp,
            max_duty_cycle,
            supports_join,
            rf_region,
            supports_32bit_fcnt,
			adr_algorithm_id
        ) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)`,
		dp.CreatedAt,
		dp.UpdatedAt,
		dp.ID,
		dp.SupportsClassB,
		dp.ClassBTimeout,
		dp.PingSlotPeriod,
		dp.PingSlotDR,
		dp.PingSlotFreq,
		dp.SupportsClassC,
		dp.ClassCTimeout,
		dp.MACVersion,
		dp.RegParamsRevision,
		dp.RXDelay1,
		dp.RXDROffset1,
		dp.RXDataRate2,
		dp.RXFreq2,
		pq.Array(dp.FactoryPresetFreqs),
		dp.MaxEIRP,
		dp.MaxDutyCycle,
		dp.SupportsJoin,
		dp.RFRegion,
		dp.Supports32bitFCnt,
		dp.ADRAlgorithmID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":     dp.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("device-profile created")

	return nil
}

// CreateDeviceProfileCache caches the given device-profile in Redis.
// The TTL of the device-profile is the same as that of the device-sessions.
func CreateDeviceProfileCache(ctx context.Context, dp DeviceProfile) error {
	key := GetRedisKey(DeviceProfileKeyTempl, dp.ID)

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(dp); err != nil {
		return errors.Wrap(err, "gob encode device-profile error")
	}

	err := RedisClient().Set(ctx, key, buf.Bytes(), deviceSessionTTL).Err()
	if err != nil {
		return errors.Wrap(err, "set device-profile error")
	}

	return nil
}

// GetDeviceProfileCache returns a cached device-profile.
func GetDeviceProfileCache(ctx context.Context, id uuid.UUID) (DeviceProfile, error) {
	var dp DeviceProfile
	key := GetRedisKey(DeviceProfileKeyTempl, id)

	val, err := RedisClient().Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return dp, ErrDoesNotExist
		}
		return dp, errors.Wrap(err, "get error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&dp)
	if err != nil {
		return dp, errors.Wrap(err, "gob decode error")
	}

	return dp, nil
}

// FlushDeviceProfileCache deletes a cached device-profile.
func FlushDeviceProfileCache(ctx context.Context, id uuid.UUID) error {
	key := GetRedisKey(DeviceProfileKeyTempl, id)

	err := RedisClient().Del(ctx, key).Err()
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	return nil
}

// GetAndCacheDeviceProfile returns the device-profile from cache
// in case available, else it will be retrieved from the database and then
// stored in cache.
func GetAndCacheDeviceProfile(ctx context.Context, db sqlx.Queryer, id uuid.UUID) (DeviceProfile, error) {
	dp, err := GetDeviceProfileCache(ctx, id)
	if err == nil {
		return dp, nil
	}

	if err != ErrDoesNotExist {
		log.WithFields(log.Fields{
			"device_profile_id": id,
		}).WithError(err).Error("get device-profile cache error")
		// we don't return as we can still fall-back onto db retrieval
	}

	dp, err = GetDeviceProfile(ctx, db, id)
	if err != nil {
		return DeviceProfile{}, errors.Wrap(err, "get device-profile error")
	}

	err = CreateDeviceProfileCache(ctx, dp)
	if err != nil {
		log.WithFields(log.Fields{
			"ctx_id":            ctx.Value(logging.ContextIDKey),
			"device_profile_id": id,
		}).WithError(err).Error("create device-profile cache error")
	}

	return dp, nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func GetDeviceProfile(ctx context.Context, db sqlx.Queryer, id uuid.UUID) (DeviceProfile, error) {
	var dp DeviceProfile

	row := db.QueryRowx(`
        select
            created_at,
            updated_at,

            device_profile_id,
            supports_class_b,
            class_b_timeout,
            ping_slot_period,
            ping_slot_dr,
            ping_slot_freq,
            supports_class_c,
            class_c_timeout,
            mac_version,
            reg_params_revision,
            rx_delay_1,
            rx_dr_offset_1,
            rx_data_rate_2,
            rx_freq_2,
            factory_preset_freqs,
            max_eirp,
            max_duty_cycle,
            supports_join,
            rf_region,
            supports_32bit_fcnt,
			adr_algorithm_id
        from device_profile
        where
            device_profile_id = $1
        `, id)

	var factoryPresetFreqs []int64

	err := row.Scan(
		&dp.CreatedAt,
		&dp.UpdatedAt,
		&dp.ID,
		&dp.SupportsClassB,
		&dp.ClassBTimeout,
		&dp.PingSlotPeriod,
		&dp.PingSlotDR,
		&dp.PingSlotFreq,
		&dp.SupportsClassC,
		&dp.ClassCTimeout,
		&dp.MACVersion,
		&dp.RegParamsRevision,
		&dp.RXDelay1,
		&dp.RXDROffset1,
		&dp.RXDataRate2,
		&dp.RXFreq2,
		pq.Array(&factoryPresetFreqs),
		&dp.MaxEIRP,
		&dp.MaxDutyCycle,
		&dp.SupportsJoin,
		&dp.RFRegion,
		&dp.Supports32bitFCnt,
		&dp.ADRAlgorithmID,
	)
	if err != nil {
		return dp, handlePSQLError(err, "select error")
	}

	for _, f := range factoryPresetFreqs {
		dp.FactoryPresetFreqs = append(dp.FactoryPresetFreqs, uint32(f))
	}

	return dp, nil
}

// UpdateDeviceProfile updates the given device-profile.
func UpdateDeviceProfile(ctx context.Context, db sqlx.Execer, dp *DeviceProfile) error {
	dp.UpdatedAt = time.Now()

	res, err := db.Exec(`
        update device_profile set
            updated_at = $2,

            supports_class_b = $3,
            class_b_timeout = $4,
            ping_slot_period = $5,
            ping_slot_dr = $6,
            ping_slot_freq = $7,
            supports_class_c = $8,
            class_c_timeout = $9,
            mac_version = $10,
            reg_params_revision = $11,
            rx_delay_1 = $12,
            rx_dr_offset_1 = $13,
            rx_data_rate_2 = $14,
            rx_freq_2 = $15,
            factory_preset_freqs = $16,
            max_eirp = $17,
            max_duty_cycle = $18,
            supports_join = $19,
            rf_region = $20,
            supports_32bit_fcnt = $21,
			adr_algorithm_id = $22
        where
            device_profile_id = $1`,
		dp.ID,
		dp.UpdatedAt,
		dp.SupportsClassB,
		dp.ClassBTimeout,
		dp.PingSlotPeriod,
		dp.PingSlotDR,
		dp.PingSlotFreq,
		dp.SupportsClassC,
		dp.ClassCTimeout,
		dp.MACVersion,
		dp.RegParamsRevision,
		dp.RXDelay1,
		dp.RXDROffset1,
		dp.RXDataRate2,
		dp.RXFreq2,
		pq.Array(dp.FactoryPresetFreqs),
		dp.MaxEIRP,
		dp.MaxDutyCycle,
		dp.SupportsJoin,
		dp.RFRegion,
		dp.Supports32bitFCnt,
		dp.ADRAlgorithmID,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"id":     dp.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("device-profile updated")
	return nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func DeleteDeviceProfile(ctx context.Context, db sqlx.Execer, id uuid.UUID) error {
	res, err := db.Exec("delete from device_profile where device_profile_id = $1", id)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affacted error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}
	log.WithFields(log.Fields{
		"id":     id,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("device-profile deleted")
	return nil
}
