package stats

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/backend/applicationserver"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/test"
	"github.com/liuhw0/lorawan"
	"github.com/liuhw0/lorawan/band"
)

type GatewayConfigurationTestSuite struct {
	suite.Suite

	backend  *test.GatewayBackend
	asClient *test.ApplicationClient

	gateway storage.Gateway
}

func (ts *GatewayConfigurationTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))
	storage.RedisClient().FlushAll(context.Background())

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))

	ts.backend = test.NewGatewayBackend()
	gateway.SetBackend(ts.backend)

	ts.gateway = storage.Gateway{
		GatewayID:        lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateGateway(context.Background(), storage.DB(), &ts.gateway))

	ts.asClient = test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(ts.asClient))
}

func (ts *GatewayConfigurationTestSuite) TestUpdate() {
	ts.T().Run("No gateway-profile", func(t *testing.T) {
		assert := require.New(t)

		assert.NoError(Handle(context.Background(), gw.GatewayStats{
			GatewayId:     ts.gateway.GatewayID[:],
			ConfigVersion: "1.2.3",
		}))

		assert.Equal(0, len(ts.backend.GatewayConfigPacketChan))
	})

	ts.T().Run("With gateway-profile", func(t *testing.T) {
		assert := require.New(t)

		gp := storage.GatewayProfile{
			Channels:      []int64{0, 1, 2},
			StatsInterval: time.Second * 30,
			ExtraChannels: []storage.ExtraChannel{
				{
					Modulation:       string(band.LoRaModulation),
					Frequency:        867100000,
					Bandwidth:        125,
					SpreadingFactors: []int64{7, 8, 9, 10, 11, 12},
				},
				{
					Modulation: string(band.FSKModulation),
					Frequency:  868800000,
					Bandwidth:  125,
					Bitrate:    50000,
				},
			},
		}

		assert.NoError(storage.CreateGatewayProfile(context.Background(), storage.DB(), &gp))

		// to work around timestamp truncation
		var err error
		gp, err = storage.GetGatewayProfile(context.Background(), storage.DB(), gp.ID)
		assert.NoError(err)

		ts.gateway.GatewayProfileID = &gp.ID
		assert.NoError(storage.UpdateGateway(context.Background(), storage.DB(), &ts.gateway))

		t.Run("No Concentratord", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(Handle(context.Background(), gw.GatewayStats{
				GatewayId:     ts.gateway.GatewayID[:],
				ConfigVersion: "1.2.3",
				MetaData:      map[string]string{},
			}))

			assert.Len(ts.backend.GatewayConfigPacketChan, 0)
		})

		t.Run("Concentratord", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(Handle(context.Background(), gw.GatewayStats{
				GatewayId:     ts.gateway.GatewayID[:],
				ConfigVersion: "1.2.3",
				MetaData: map[string]string{
					"concentratord_version": "3.3.0",
				},
			}))

			gwConfig := <-ts.backend.GatewayConfigPacketChan
			assert.Equal(gw.GatewayConfiguration{
				Version:       gp.GetVersion(),
				GatewayId:     ts.gateway.GatewayID[:],
				StatsInterval: ptypes.DurationProto(time.Second * 30),
				Channels: []*gw.ChannelConfiguration{
					{
						Frequency:  868100000,
						Modulation: common.Modulation_LORA,
						ModulationConfig: &gw.ChannelConfiguration_LoraModulationConfig{
							LoraModulationConfig: &gw.LoRaModulationConfig{
								Bandwidth:        125,
								SpreadingFactors: []uint32{7, 8, 9, 10, 11, 12},
							},
						},
					},
					{
						Frequency:  868300000,
						Modulation: common.Modulation_LORA,
						ModulationConfig: &gw.ChannelConfiguration_LoraModulationConfig{
							LoraModulationConfig: &gw.LoRaModulationConfig{
								Bandwidth:        125,
								SpreadingFactors: []uint32{7, 8, 9, 10, 11, 12},
							},
						},
					},
					{
						Frequency:  868500000,
						Modulation: common.Modulation_LORA,
						ModulationConfig: &gw.ChannelConfiguration_LoraModulationConfig{
							LoraModulationConfig: &gw.LoRaModulationConfig{
								Bandwidth:        125,
								SpreadingFactors: []uint32{7, 8, 9, 10, 11, 12},
							},
						},
					},
					{
						Frequency:  867100000,
						Modulation: common.Modulation_LORA,
						ModulationConfig: &gw.ChannelConfiguration_LoraModulationConfig{
							LoraModulationConfig: &gw.LoRaModulationConfig{
								Bandwidth:        125,
								SpreadingFactors: []uint32{7, 8, 9, 10, 11, 12},
							},
						},
					},
					{
						Frequency:  868800000,
						Modulation: common.Modulation_FSK,
						ModulationConfig: &gw.ChannelConfiguration_FskModulationConfig{
							FskModulationConfig: &gw.FSKModulationConfig{
								Bandwidth: 125,
								Bitrate:   50000,
							},
						},
					},
				},
			}, gwConfig)
		})

	})
}

func TestGatewayConfigurationUpdate(t *testing.T) {
	suite.Run(t, new(GatewayConfigurationTestSuite))
}

type GatewayStatsTestSuite struct {
	suite.Suite

	gateway  storage.Gateway
	asClient *test.ApplicationClient
}

func (ts *GatewayStatsTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))
	storage.RedisClient().FlushAll(context.Background())

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))

	ts.gateway = storage.Gateway{
		GatewayID:        lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateGateway(context.Background(), storage.DB(), &ts.gateway))

	ts.asClient = test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(ts.asClient))
}

func (ts *GatewayStatsTestSuite) TestStats() {
	assert := require.New(ts.T())

	now := time.Now()
	statsID, err := uuid.NewV4()
	assert.NoError(err)

	stats := gw.GatewayStats{
		GatewayId: ts.gateway.GatewayID[:],
		StatsId:   statsID[:],
		Location: &common.Location{
			Latitude:  1.123,
			Longitude: 1.124,
			Altitude:  15.3,
		},
		RxPacketsReceived:   11,
		RxPacketsReceivedOk: 9,
		TxPacketsReceived:   13,
		TxPacketsEmitted:    10,
		TxPacketsPerFrequency: map[uint32]uint32{
			868100000: 10,
		},
		RxPacketsPerFrequency: map[uint32]uint32{
			868300000: 9,
		},
		TxPacketsPerModulation: []*gw.PerModulationCount{
			{
				Modulation: &gw.Modulation{
					Parameters: &gw.Modulation_Lora{
						Lora: &gw.LoRaModulationInfo{
							Bandwidth:       125,
							SpreadingFactor: 8,
							CodeRate:        "4/5",
						},
					},
				},
				Count: 10,
			},
		},

		RxPacketsPerModulation: []*gw.PerModulationCount{
			{
				Modulation: &gw.Modulation{
					Parameters: &gw.Modulation_Lora{
						Lora: &gw.LoRaModulationInfo{
							Bandwidth:       125,
							SpreadingFactor: 10,
							CodeRate:        "4/5",
						},
					},
				},
				Count: 9,
			},
		},
		TxPacketsPerStatus: map[string]uint32{
			"OK":       10,
			"TOO_LATE": 3,
		},
		MetaData: map[string]string{
			"foo": "bar",
		},
	}
	stats.Time, _ = ptypes.TimestampProto(now)
	assert.NoError(Handle(context.Background(), stats))

	asReq := <-ts.asClient.HandleGatewayStatsChan
	assert.Equal(as.HandleGatewayStatsRequest{
		GatewayId:           stats.GatewayId,
		StatsId:             stats.StatsId,
		Time:                stats.Time,
		Location:            stats.Location,
		RxPacketsReceived:   stats.RxPacketsReceived,
		RxPacketsReceivedOk: stats.RxPacketsReceivedOk,
		TxPacketsReceived:   stats.TxPacketsReceived,
		TxPacketsEmitted:    stats.TxPacketsEmitted,
		TxPacketsPerFrequency: map[uint32]uint32{
			868100000: 10,
		},
		RxPacketsPerFrequency: map[uint32]uint32{
			868300000: 9,
		},
		TxPacketsPerDr: map[uint32]uint32{
			4: 10,
		},
		RxPacketsPerDr: map[uint32]uint32{
			2: 9,
		},
		TxPacketsPerStatus: map[string]uint32{
			"OK":       10,
			"TOO_LATE": 3,
		},
		Metadata: map[string]string{
			"foo": "bar",
		},
	}, asReq)
}

func TestGatewayStats(t *testing.T) {
	suite.Run(t, new(GatewayStatsTestSuite))
}
