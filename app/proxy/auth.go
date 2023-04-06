package proxy

import "fmt"

type (
	authorization struct {
		via    authzVia
		from   authzFrom
		policy authzPolicy
	}

	authzVia int

	authzFrom int

	authzPolicy int
)

const (
	nullVia authzVia = iota
	accessToken
)

const (
	nullFrom authzFrom = iota
	header
	cookie
)

const (
	nullPolicy authzPolicy = iota
	allowed
	permitted
	enforced
	forbidden
	custom
	partner
)

var (
	authzViaStrings    = []string{"null", "access token"}
	authzFromStrings   = []string{"null", "header", "cookie"}
	authzPolicyStrings = []string{"null", "allowed", "permitted", "enforced", "forbidden", "custom", "partner"}
)

func (a authorization) String() string {
	return fmt.Sprintf("(via: %s, from: %s, policy: %s)",
		authzViaStrings[a.via],
		authzFromStrings[a.from],
		authzPolicyStrings[a.policy],
	)
}

func (a *authorization) validate() error {
	if a.policy == nullPolicy {
		return ErrNilPolicy
	}

	if a.policy == permitted || a.policy == enforced {
		if a.from == nullFrom {
			return ErrNilFrom
		}

		if a.via == nullVia {
			return ErrNilVia
		}
	}

	return nil
}

func parseAuthorization(r *configRoute) (authorization, error) {
	var (
		av authzVia
		af authzFrom
		ap authzPolicy
	)

	if r.Authorization != nil {
		if r.Authorization.Via != nil {
			if *r.Authorization.Via == "token" {
				av = accessToken
			} else {
				return authorization{}, fmt.Errorf("via %q is not valid", *r.Authorization.Via)
			}
		}

		if r.Authorization.From != nil {
			switch *r.Authorization.From {
			case "header":
				af = header
			case "cookie":
				af = cookie
			default:
				return authorization{}, fmt.Errorf("from %q is not valid", *r.Authorization.From)
			}
		}

		if r.Authorization.Policy != nil {
			switch *r.Authorization.Policy {
			case "allowed":
				ap = allowed
			case "permitted":
				ap = permitted
			case "enforced":
				ap = enforced
			case "forbidden":
				ap = forbidden
			case "custom":
				ap = custom
			case "partner":
				ap = partner
			default:
				return authorization{}, fmt.Errorf("policy %q is not valid", *r.Authorization.Policy)
			}
		}
	}

	return authorization{
		via:    av,
		from:   af,
		policy: ap,
	}, nil
}
