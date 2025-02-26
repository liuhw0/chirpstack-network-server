package downlink

import (
	"time"

	"github.com/pkg/errors"

	"github.com/liuhw0/chirpstack-network-server/v3/internal/config"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/downlink/data"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/downlink/join"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/downlink/multicast"
	"github.com/liuhw0/chirpstack-network-server/v3/internal/downlink/proprietary"
)

var (
	schedulerBatchSize = 100
	schedulerInterval  time.Duration
)

// Setup sets up the downlink.
func Setup(conf config.Config) error {
	nsConfig := conf.NetworkServer
	schedulerInterval = nsConfig.Scheduler.SchedulerInterval

	if err := data.Setup(conf); err != nil {
		return errors.Wrap(err, "setup downlink/data error")
	}

	if err := join.Setup(conf); err != nil {
		return errors.Wrap(err, "setup downlink/join error")
	}

	if err := multicast.Setup(conf); err != nil {
		return errors.Wrap(err, "setup downlink/multicast error")
	}

	if err := proprietary.Setup(conf); err != nil {
		return errors.Wrap(err, "setup downlink/proprietary error")
	}

	return nil
}
