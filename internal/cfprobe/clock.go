package cfprobe

import (
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxCalibrationAge        = 24 * time.Hour
	dateCalibrationThreshold = 20 * time.Second
	responseDateHeader       = "Date"
)

// ReportTime describes the local wall clock and the latest independent clock
// calibration. Pointer fields intentionally serialize as null before the first
// successful calibration.
type ReportTime struct {
	LocalTS     int64   `json:"local_ts"`
	AccurateTS  *int64  `json:"accurate_ts"`
	OffsetMS    *int64  `json:"offset_ms"`
	Source      *string `json:"source"`
	RoundTripMS *uint64 `json:"round_trip_ms"`
	SampleAgeMS *uint64 `json:"sample_age_ms"`
}

type calibratedClock struct {
	mu          sync.Mutex
	calibration *clockCalibration
}

type clockCalibration struct {
	source           string
	anchor           time.Time
	accurateAtAnchor int64
	roundTripMS      uint64
}

func newDateCalibration(dateTime int64, roundTrip time.Duration, anchor time.Time) clockCalibration {
	roundTripMS := durationMilliseconds(roundTrip)
	// The HTTP Date header is generated close to response transmission. With
	// one server timestamp, half of the request RTT is the best available
	// estimate of the response's remaining travel time.
	halfRoundTrip := roundTripMS/2 + roundTripMS%2
	return clockCalibration{
		source:           "date",
		anchor:           anchor,
		accurateAtAnchor: addMilliseconds(dateTime, halfRoundTrip),
		roundTripMS:      roundTripMS,
	}
}

func (c clockCalibration) snapshot(now time.Time) ReportTime {
	age := now.Sub(c.anchor)
	if age < 0 {
		age = 0
	}
	ageMS := durationMilliseconds(age)
	accurateTS := c.timestamp(now)
	localTS := now.UnixMilli()
	offsetMS := saturatingSubtract(accurateTS, localTS)
	source := c.source
	roundTripMS := c.roundTripMS
	return ReportTime{
		LocalTS:     localTS,
		AccurateTS:  &accurateTS,
		OffsetMS:    &offsetMS,
		Source:      &source,
		RoundTripMS: &roundTripMS,
		SampleAgeMS: &ageMS,
	}
}

func (c clockCalibration) timestamp(at time.Time) int64 {
	deltaMilliseconds := int64(at.Sub(c.anchor) / time.Millisecond)
	return saturatingAdd(c.accurateAtAnchor, deltaMilliseconds)
}

func (c *calibratedClock) snapshot(now time.Time) ReportTime {
	c.mu.Lock()
	defer c.mu.Unlock()

	calibration := c.calibrationAt(now, false)
	if calibration == nil {
		return ReportTime{LocalTS: now.UnixMilli()}
	}
	return calibration.snapshot(now)
}

// timestamp maps an observation time onto the calibrated Unix timeline. A
// fresh calibration may be used retroactively so samples collected shortly
// before the Date response can still be corrected before they are reported.
func (c *calibratedClock) timestamp(at time.Time) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	calibration := c.calibrationAt(at, true)
	if calibration == nil {
		return at.UnixMilli(), false
	}
	return calibration.timestamp(at), true
}

// correctLocalTimestamp applies the calibrated offset at observedAt to an
// absolute timestamp supplied by the local OS, such as boot_time.
func (c *calibratedClock) correctLocalTimestamp(localTimestamp int64, observedAt time.Time) int64 {
	accurateAt, ok := c.timestamp(observedAt)
	if !ok {
		return localTimestamp
	}
	offset := saturatingSubtract(accurateAt, observedAt.UnixMilli())
	return saturatingAdd(localTimestamp, offset)
}

func (c *calibratedClock) calibrationAt(at time.Time, allowRetroactive bool) *clockCalibration {
	if calibrationUsable(c.calibration, at, allowRetroactive) {
		return c.calibration
	}
	return nil
}

func calibrationUsable(calibration *clockCalibration, at time.Time, allowRetroactive bool) bool {
	if calibration == nil {
		return false
	}
	age := at.Sub(calibration.anchor)
	if age < 0 {
		return allowRetroactive && age >= -maxCalibrationAge
	}
	return age <= maxCalibrationAge
}

func (c *calibratedClock) updateDate(dateTime int64, roundTrip time.Duration, anchor time.Time) (ReportTime, bool) {
	calibration := newDateCalibration(dateTime, roundTrip, anchor)
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.calibrationAt(anchor, false)
	if current != nil {
		currentTimestamp := current.timestamp(anchor)
		if absoluteDifference(currentTimestamp, calibration.accurateAtAnchor) <= uint64(dateCalibrationThreshold/time.Millisecond) {
			return current.snapshot(now), false
		}
	}
	c.calibration = &calibration
	return calibration.snapshot(now), true
}

func durationMilliseconds(duration time.Duration) uint64 {
	if duration <= 0 {
		return 0
	}
	return uint64(duration / time.Millisecond)
}

func addMilliseconds(timestamp int64, milliseconds uint64) int64 {
	if milliseconds > math.MaxInt64 || timestamp > math.MaxInt64-int64(milliseconds) {
		return math.MaxInt64
	}
	return timestamp + int64(milliseconds)
}

func saturatingSubtract(left, right int64) int64 {
	if right > 0 && left < math.MinInt64+right {
		return math.MinInt64
	}
	if right < 0 && left > math.MaxInt64+right {
		return math.MaxInt64
	}
	return left - right
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	if right < 0 && left < math.MinInt64-right {
		return math.MinInt64
	}
	return left + right
}

func absoluteDifference(left, right int64) uint64 {
	if left >= right {
		return uint64(left) - uint64(right)
	}
	return uint64(right) - uint64(left)
}

func responseDateTime(headers http.Header) (int64, bool) {
	raw := strings.TrimSpace(headers.Get(responseDateHeader))
	if raw == "" {
		return 0, false
	}
	parsed, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	return parsed.UnixMilli(), true
}
