package app

import (
	"time"

	"github.com/spf13/viper"
)

func providerPollingConfigFromConfig(cfg *viper.Viper) ProviderPollingConfig {
	poll := DefaultProviderPollingConfig()
	if cfg == nil {
		return poll
	}
	if interval := cfg.GetDuration("provider-sync-interval"); interval > 0 {
		poll.Interval = interval
	}
	if min := cfg.GetDuration("provider-sync-min-backoff"); min > 0 {
		poll.MinBackoff = min
	}
	if max := cfg.GetDuration("provider-sync-max-backoff"); max > 0 {
		poll.MaxBackoff = max
	}
	if poll.MinBackoff == 0 {
		poll.MinBackoff = 5 * time.Second
	}
	if poll.MaxBackoff == 0 {
		poll.MaxBackoff = time.Minute
	}
	return poll
}
