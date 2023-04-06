package proxy

import "time"

type rateLimit struct {
	enabled  bool
	limit    uint64
	duration time.Duration
}

func (c *rateLimit) parse(r *configRoute) {
	if r.RateLimit == nil {
		return
	}

	if r.RateLimit.Enabled != nil {
		c.enabled = *r.RateLimit.Enabled
	}

	if r.RateLimit.Limit != nil {
		c.limit = *r.RateLimit.Limit
	}

	if r.RateLimit.Duration != nil {
		c.duration = *r.RateLimit.Duration
	}
}

func (c *rateLimit) validate() error {
	if !c.enabled {
		return nil
	}

	if c.limit == 0 {
		return ErrInvalidRateLimit
	}

	if c.duration == 0 {
		return ErrInvalidRateLimitDuration
	}

	return nil
}
