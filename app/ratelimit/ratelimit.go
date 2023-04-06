package ratelimit

import (
	"context"
	"time"
)

type (
	Strategy interface {
		Run(context.Context, Request) (Result, error)
	}

	State uint8

	Request struct {
		Key      string
		Limit    uint64
		Duration time.Duration
	}

	Result struct {
		State         State
		ExpiresAt     time.Time
		TotalRequests uint64
	}
)

const (
	Deny State = iota
	Allow
)

var stateStr = []string{"Deny", "Allow"}
