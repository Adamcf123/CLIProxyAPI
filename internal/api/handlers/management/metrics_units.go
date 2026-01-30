package management

import "math"

func secondsToMillisInt(sec float64) int64 {
	return int64(math.Round(sec * 1000))
}
