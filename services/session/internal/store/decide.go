package store

import "time"

type decision int

const (
	decisionRotate decision = iota
	decisionReuse
	decisionReject
)

func decide(used, revoked bool, sessionStatus string, tokenExp, sessionExp, now time.Time) decision {
	if used {
		return decisionReuse
	}
	if revoked || sessionStatus != "active" || !tokenExp.After(now) || !sessionExp.After(now) {
		return decisionReject
	}
	return decisionRotate
}
