package server

import "time"

func now() time.Time {
	return time.Now().UTC()
}
