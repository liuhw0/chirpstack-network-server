package testsuite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/uplink"
	"github.com/liuhw0/lorawan"
)

type DeviceGatewayRXInfoSetTestSuite struct {
	IntegrationTestSuite
}

func (ts *DeviceGatewayRXInfoSetTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()

	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}})
	ts.CreateDevice(storage.Device{DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}})
	ts.CreateDeviceSession(storage.DeviceSession{})
}

// this tests that on uplink data, the device gateway rx-info set has been
// stored in redis
func (ts *DeviceGatewayRXInfoSetTestSuite) TestDeviceGatewayRXInfoSetHasBeenStored() {
	assert := require.New(ts.T())

	txInfo := gw.UplinkTXInfo{
		Frequency:  868300000,
		Modulation: common.Modulation_LORA,
		ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				SpreadingFactor: 10,
				Bandwidth:       125,
			},
		},
	}
	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.GatewayID[:],
		Rssi:      -60,
		LoraSnr:   5.5,
	}

	assert.Nil(uplink.HandleUplinkFrame(context.Background(), ts.GetUplinkFrameForFRMPayload(rxInfo, txInfo, lorawan.UnconfirmedDataUp, 10, []byte{1, 2, 3, 4})))

	rxInfoSet, err := storage.GetDeviceGatewayRXInfoSet(context.Background(), ts.Device.DevEUI)
	assert.Nil(err)

	assert.Equal(storage.DeviceGatewayRXInfoSet{
		DevEUI: ts.Device.DevEUI,
		DR:     2,
		Items: []storage.DeviceGatewayRXInfo{
			{
				GatewayID: ts.Gateway.GatewayID,
				RSSI:      -60,
				LoRaSNR:   5.5,
			},
		},
	}, rxInfoSet)
}

func TestDeviceGatewayRXInfoSet(t *testing.T) {
	suite.Run(t, new(DeviceGatewayRXInfoSetTestSuite))
}
