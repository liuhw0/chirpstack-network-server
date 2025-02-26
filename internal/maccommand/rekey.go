package maccommand

import (
	"context"
	"fmt"

	"github.com/liuhw0/lorawan"
	log "github.com/sirupsen/logrus"

	"github.com/liuhw0/chirpstack-network-server/v3/internal/logging"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
)

const servLoRaWANVersionMinor uint8 = 1

func handleRekeyInd(ctx context.Context, ds *storage.DeviceSession, block storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) != 1 {
		return nil, fmt.Errorf("exactly one mac-command expected, got %d", len(block.MACCommands))
	}

	pl, ok := block.MACCommands[0].Payload.(*lorawan.RekeyIndPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.RekeyIndPayload, got %T", block.MACCommands[0].Payload)
	}

	respPL := lorawan.RekeyConfPayload{
		ServLoRaWANVersion: lorawan.Version{
			Minor: servLoRaWANVersionMinor,
		},
	}

	if servLoRaWANVersionMinor > pl.DevLoRaWANVersion.Minor {
		respPL.ServLoRaWANVersion.Minor = pl.DevLoRaWANVersion.Minor
	}

	log.WithFields(log.Fields{
		"dev_eui":                    ds.DevEUI,
		"dev_lorawan_version_minor":  pl.DevLoRaWANVersion.Minor,
		"serv_lorawan_version_minor": servLoRaWANVersionMinor,
		"ctx_id":                     ctx.Value(logging.ContextIDKey),
	}).Info("rekey_ind received")

	return []storage.MACCommandBlock{
		{
			CID: lorawan.RekeyConf,
			MACCommands: storage.MACCommands{
				{
					CID:     lorawan.RekeyConf,
					Payload: &respPL,
				},
			},
		},
	}, nil
}
