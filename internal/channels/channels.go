package channels

import (
	"github.com/liuhw0/chirpstack-network-server/v3/internal/band"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
	"github.com/liuhw0/lorawan"
)

// HandleChannelReconfigure handles the reconfiguration of active channels
// on the node. This is needed in case only a sub-set of channels is used
// (e.g. for the US band) or when a reconfiguration of active channels
// happens.
func HandleChannelReconfigure(ds storage.DeviceSession) ([]storage.MACCommandBlock, error) {
	payloads := band.Band().GetLinkADRReqPayloadsForEnabledUplinkChannelIndices(ds.EnabledUplinkChannels)
	if len(payloads) == 0 {
		return nil, nil
	}

	payloads[len(payloads)-1].TXPower = uint8(ds.TXPowerIndex)
	payloads[len(payloads)-1].DataRate = uint8(ds.DR)
	payloads[len(payloads)-1].Redundancy.NbRep = ds.NbTrans

	block := storage.MACCommandBlock{
		CID: lorawan.LinkADRReq,
	}
	for i := range payloads {
		block.MACCommands = append(block.MACCommands, lorawan.MACCommand{
			CID:     lorawan.LinkADRReq,
			Payload: &payloads[i],
		})
	}

	return []storage.MACCommandBlock{block}, nil
}
