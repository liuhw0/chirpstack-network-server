package maccommand

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/liuhw0/chirpstack-network-server/v3/internal/logging"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/storage"
	"github.com/liuhw0/lorawan"
	"github.com/liuhw0/lorawan/band"
)

// RequestNewChannels creates or modifies the non-common bi-directional
// channels in case of changes between the current and wanted channels.
// To avoid generating mac-command blocks which can't be sent, and to
// modify the channels in multiple batches, the max number of channels to
// modify must be given. In case of no changes, nil is returned.
func RequestNewChannels(devEUI lorawan.EUI64, maxChannels int, currentChannels, wantedChannels map[int]band.Channel) *storage.MACCommandBlock {
	var out []lorawan.MACCommand

	// sort by channel index
	var wantedChannelNumbers []int
	for i := range wantedChannels {
		wantedChannelNumbers = append(wantedChannelNumbers, i)
	}
	sort.Ints(wantedChannelNumbers)

	for _, i := range wantedChannelNumbers {
		wanted := wantedChannels[i]
		current, ok := currentChannels[i]
		if !ok || current.Frequency != wanted.Frequency || current.MinDR != wanted.MinDR || current.MaxDR != wanted.MaxDR {
			out = append(out, lorawan.MACCommand{
				CID: lorawan.NewChannelReq,
				Payload: &lorawan.NewChannelReqPayload{
					ChIndex: uint8(i),
					Freq:    uint32(wanted.Frequency),
					MinDR:   uint8(wanted.MinDR),
					MaxDR:   uint8(wanted.MaxDR),
				},
			})
		}
	}

	if len(out) > maxChannels {
		out = out[0:maxChannels]
	}

	if len(out) == 0 {
		return nil
	}

	return &storage.MACCommandBlock{
		CID:         lorawan.NewChannelReq,
		MACCommands: storage.MACCommands(out),
	}
}

func handleNewChannelAns(ctx context.Context, ds *storage.DeviceSession, block storage.MACCommandBlock, pending *storage.MACCommandBlock) ([]storage.MACCommandBlock, error) {
	if len(block.MACCommands) == 0 {
		return nil, errors.New("at least 1 mac-command expected, got none")
	}

	if pending == nil || len(pending.MACCommands) == 0 {
		return nil, errors.New("expected pending mac-command")
	}

	if len(block.MACCommands) != len(pending.MACCommands) {
		return nil, fmt.Errorf("received %d mac-command answers, but requested %d", len(block.MACCommands), len(pending.MACCommands))
	}

	for i := range block.MACCommands {
		pl, ok := block.MACCommands[i].Payload.(*lorawan.NewChannelAnsPayload)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.NewChannelAnsPayload, got %T", block.MACCommands[i].Payload)
		}

		pendingPL, ok := pending.MACCommands[i].Payload.(*lorawan.NewChannelReqPayload)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.NewChannelReqPayload, got %T", pending.MACCommands[i].Payload)
		}

		if pl.ChannelFrequencyOK && pl.DataRateRangeOK {
			// reset the error counter
			delete(ds.MACCommandErrorCount, lorawan.NewChannelAns)

			ds.ExtraUplinkChannels[int(pendingPL.ChIndex)] = band.Channel{
				Frequency: pendingPL.Freq,
				MinDR:     int(pendingPL.MinDR),
				MaxDR:     int(pendingPL.MaxDR),
			}

			var found bool
			for _, i := range ds.EnabledUplinkChannels {
				if i == int(pendingPL.ChIndex) {
					found = true
				}
			}
			if !found {
				ds.EnabledUplinkChannels = append(ds.EnabledUplinkChannels, int(pendingPL.ChIndex))
			}

			log.WithFields(log.Fields{
				"frequency": pendingPL.Freq,
				"channel":   pendingPL.ChIndex,
				"min_dr":    pendingPL.MinDR,
				"max_dr":    pendingPL.MaxDR,
				"ctx_id":    ctx.Value(logging.ContextIDKey),
				"dev_eui":   ds.DevEUI,
			}).Info("new_channel request acknowledged")
		} else {
			// increase error counter
			ds.MACCommandErrorCount[lorawan.NewChannelAns]++

			log.WithFields(log.Fields{
				"frequency":            pendingPL.Freq,
				"channel":              pendingPL.ChIndex,
				"min_dr":               pendingPL.MinDR,
				"max_dr":               pendingPL.MaxDR,
				"data_rate_range_ok":   pl.DataRateRangeOK,
				"channel_frequency_ok": pl.ChannelFrequencyOK,
				"ctx_id":               ctx.Value(logging.ContextIDKey),
				"dev_eui":              ds.DevEUI,
			}).Warning("new_channel request not acknowledged")
		}
	}

	return nil, nil
}
