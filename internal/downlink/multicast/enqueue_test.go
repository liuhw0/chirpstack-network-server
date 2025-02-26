package multicast

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/liuhw0/lorawan"

	"github.com/liuhw0/chirpstack-network-server/v3/internal/config"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/test"
)

type EnqueueQueueItemTestCase struct {
	suite.Suite

	MulticastGroup storage.MulticastGroup
	Devices        []storage.Device
	Gateways       []storage.Gateway

	tx *storage.TxLogger
}

func (ts *EnqueueQueueItemTestCase) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))
}

func (ts *EnqueueQueueItemTestCase) TearDownTest() {
	ts.tx.Rollback()
}

func (ts *EnqueueQueueItemTestCase) SetupTest() {
	assert := require.New(ts.T())
	var err error
	ts.tx, err = storage.DB().Beginx()
	assert.NoError(err)
	storage.RedisClient().FlushAll(context.Background())

	var sp storage.ServiceProfile
	var rp storage.RoutingProfile

	assert.NoError(storage.CreateServiceProfile(context.Background(), ts.tx, &sp))
	assert.NoError(storage.CreateRoutingProfile(context.Background(), ts.tx, &rp))

	ts.MulticastGroup = storage.MulticastGroup{
		GroupType:        storage.MulticastGroupC,
		MCAddr:           lorawan.DevAddr{1, 2, 3, 4},
		MCNwkSKey:        lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		Frequency:        868100000,
		FCnt:             11,
		DR:               3,
		ServiceProfileID: sp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateMulticastGroup(context.Background(), ts.tx, &ts.MulticastGroup))

	ts.Gateways = []storage.Gateway{
		{
			GatewayID:        lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
			RoutingProfileID: rp.ID,
		},
		{
			GatewayID:        lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
			RoutingProfileID: rp.ID,
		},
	}
	for i := range ts.Gateways {
		assert.NoError(storage.CreateGateway(context.Background(), ts.tx, &ts.Gateways[i]))
	}

	var dp storage.DeviceProfile

	assert.NoError(storage.CreateDeviceProfile(context.Background(), ts.tx, &dp))

	ts.Devices = []storage.Device{
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
			DeviceProfileID:  dp.ID,
		},
		{
			DevEUI:           lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
			ServiceProfileID: sp.ID,
			RoutingProfileID: rp.ID,
			DeviceProfileID:  dp.ID,
		},
	}
	for i := range ts.Devices {
		assert.NoError(storage.CreateDevice(context.Background(), ts.tx, &ts.Devices[i]))
		assert.NoError(storage.AddDeviceToMulticastGroup(context.Background(), ts.tx, ts.Devices[i].DevEUI, ts.MulticastGroup.ID))
		assert.NoError(storage.SaveDeviceGatewayRXInfoSet(context.Background(), storage.DeviceGatewayRXInfoSet{
			DevEUI: ts.Devices[i].DevEUI,
			DR:     3,
			Items: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: ts.Gateways[i].GatewayID,
					RSSI:      50,
					LoRaSNR:   5,
				},
			},
		}))
	}
}

func (ts *EnqueueQueueItemTestCase) TestInvalidFCnt() {
	assert := require.New(ts.T())
	assert.Equal(storage.MulticastGroupC, ts.MulticastGroup.GroupType)

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             10,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.Equal(ErrInvalidFCnt, EnqueueQueueItem(context.Background(), ts.tx, qi))
}

func (ts *EnqueueQueueItemTestCase) TestClassC() {
	assert := require.New(ts.T())
	assert.Equal(storage.MulticastGroupC, ts.MulticastGroup.GroupType)

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             11,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(EnqueueQueueItem(context.Background(), ts.tx, qi))

	items, err := storage.GetMulticastQueueItemsForMulticastGroup(context.Background(), ts.tx, ts.MulticastGroup.ID)
	assert.NoError(err)
	assert.Len(items, 2)

	assert.NotEqual(items[0].GatewayID, items[1].GatewayID)
	assert.Nil(items[0].EmitAtTimeSinceGPSEpoch)
	assert.Nil(items[1].EmitAtTimeSinceGPSEpoch)

	lockDuration := config.C.NetworkServer.Scheduler.ClassC.DeviceDownlinkLockDuration
	assert.EqualValues(math.Abs(float64(items[0].ScheduleAt.Sub(items[1].ScheduleAt))), lockDuration)

	mg, err := storage.GetMulticastGroup(context.Background(), ts.tx, ts.MulticastGroup.ID, false)
	assert.NoError(err)
	assert.Equal(qi.FCnt+1, mg.FCnt)
}

func (ts *EnqueueQueueItemTestCase) TestClassB() {
	assert := require.New(ts.T())

	ts.MulticastGroup.PingSlotPeriod = 16
	ts.MulticastGroup.GroupType = storage.MulticastGroupB
	assert.NoError(storage.UpdateMulticastGroup(context.Background(), ts.tx, &ts.MulticastGroup))

	qi := storage.MulticastQueueItem{
		MulticastGroupID: ts.MulticastGroup.ID,
		FCnt:             11,
		FPort:            2,
		FRMPayload:       []byte{1, 2, 3, 4},
	}
	assert.NoError(EnqueueQueueItem(context.Background(), ts.tx, qi))

	items, err := storage.GetMulticastQueueItemsForMulticastGroup(context.Background(), ts.tx, ts.MulticastGroup.ID)
	assert.NoError(err)
	assert.Len(items, 2)
	assert.NotNil(items[0].EmitAtTimeSinceGPSEpoch)
	assert.NotNil(items[1].EmitAtTimeSinceGPSEpoch)
	assert.NotEqual(items[0].ScheduleAt, items[1].ScheduleAt)

	mg, err := storage.GetMulticastGroup(context.Background(), ts.tx, ts.MulticastGroup.ID, false)
	assert.NoError(err)
	assert.Equal(qi.FCnt+1, mg.FCnt)
}

func TestEnqueueQueueItem(t *testing.T) {
	suite.Run(t, new(EnqueueQueueItemTestCase))
}
