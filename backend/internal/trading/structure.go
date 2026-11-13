package trading

import (
	"fmt"
	"sort"
	"time"
)

// RulesVersion identifies the structure rules below.
const RulesVersion = "structure-1.0"

// Swing is a confirmed fractal pivot: a high (low) strictly above (below) the
// highs (lows) of swingWidth bars on each side.
type Swing struct {
	Kind  string // "high" | "low"
	Price float64
	At    time.Time
	Index int
}

// Break is a close beyond the most recent opposite swing. A break in the
// direction of the prevailing structure is a BOS (break of structure); a break
// against it is a CHoCH (change of character).
type Break struct {
	Kind      string // "BOS" | "CHoCH"
	Direction string // "bullish" | "bearish"
	Price     float64
	At        time.Time
	Index     int
}

// Zone is an order block or fair value gap.
type Zone struct {
	ID        string
	Kind      string // "order_block" | "fair_value_gap"
	Direction string
	Status    string // fresh | tested | mitigated | invalidated | expired
	Top       float64
	Bottom    float64
	From      time.Time
	To        *time.Time
	Index     int
}

// Structure is the full market-structure read of a candle series.
type Structure struct {
	Swings []Swing
	Breaks []Break
	Zones  []Zone
	Trend  string // direction of the latest break, "" when none
}

const (
	swingWidth  = 2
	zoneExpiry  = 60 // bars after which an untouched zone expires
	maxZonesOut = 12
)

// AnalyzeStructure applies the documented rules:
//
//  1. Swings: fractal pivots with swingWidth bars on each side.
//  2. Breaks: a close above the latest unbroken swing high is bullish, below
//     the latest unbroken swing low bearish; BOS if it agrees with the prior
//     break direction (or is the first), CHoCH otherwise.
//  3. Order block: the last opposite-colored candle before the impulse that
//     produced a break (bullish OB = last down candle before a bullish break).
//  4. Fair value gap: a three-candle gap where candle[i-2].H < candle[i].L
//     (bullish) or candle[i-2].L > candle[i].H (bearish).
//  5. Zone status, judged on later candles: invalidated when a close goes
//     through the far edge, mitigated when price trades through the midpoint,
//     tested when price touches the zone, expired after zoneExpiry bars
//     untouched, fresh otherwise.
func AnalyzeStructure(candles []Candle) Structure {
	var s Structure
	n := len(candles)
	for i := swingWidth; i < n-swingWidth; i++ {
		isHigh, isLow := true, true
		for j := i - swingWidth; j <= i+swingWidth; j++ {
			if j == i {
				continue
			}
			if candles[j].H >= candles[i].H {
				isHigh = false
			}
			if candles[j].L <= candles[i].L {
				isLow = false
			}
		}
		if isHigh {
			s.Swings = append(s.Swings, Swing{Kind: "high", Price: candles[i].H, At: candles[i].T, Index: i})
		}
		if isLow {
			s.Swings = append(s.Swings, Swing{Kind: "low", Price: candles[i].L, At: candles[i].T, Index: i})
		}
	}

	// Breaks: walk candles, tracking the latest confirmed (index+width < i)
	// swing high/low that has not been broken yet.
	lastHigh, lastLow := -1, -1
	swingIdx := 0
	for i := 0; i < n; i++ {
		for swingIdx < len(s.Swings) && s.Swings[swingIdx].Index+swingWidth < i {
			if s.Swings[swingIdx].Kind == "high" {
				lastHigh = swingIdx
			} else {
				lastLow = swingIdx
			}
			swingIdx++
		}
		if lastHigh >= 0 && candles[i].C > s.Swings[lastHigh].Price {
			s.addBreak("bullish", s.Swings[lastHigh].Price, candles[i], i)
			s.addOrderBlock(candles, i, "bullish")
			lastHigh = -1
		}
		if lastLow >= 0 && candles[i].C < s.Swings[lastLow].Price {
			s.addBreak("bearish", s.Swings[lastLow].Price, candles[i], i)
			s.addOrderBlock(candles, i, "bearish")
			lastLow = -1
		}
	}

	for i := 2; i < n; i++ {
		if candles[i-2].H < candles[i].L {
			s.Zones = append(s.Zones, Zone{Kind: "fair_value_gap", Direction: "bullish",
				Top: candles[i].L, Bottom: candles[i-2].H, From: candles[i-1].T, Index: i})
		}
		if candles[i-2].L > candles[i].H {
			s.Zones = append(s.Zones, Zone{Kind: "fair_value_gap", Direction: "bearish",
				Top: candles[i-2].L, Bottom: candles[i].H, From: candles[i-1].T, Index: i})
		}
	}

	sort.SliceStable(s.Zones, func(a, b int) bool { return s.Zones[a].Index < s.Zones[b].Index })
	for k := range s.Zones {
		s.Zones[k].Status, s.Zones[k].To = zoneStatus(s.Zones[k], candles)
		s.Zones[k].ID = fmt.Sprintf("%s-%s-%d", s.Zones[k].Kind, s.Zones[k].Direction, s.Zones[k].From.Unix())
	}
	if len(s.Breaks) > 0 {
		s.Trend = s.Breaks[len(s.Breaks)-1].Direction
	}
	return s
}

func (s *Structure) addBreak(direction string, price float64, c Candle, i int) {
	kind := "BOS"
	if len(s.Breaks) > 0 && s.Breaks[len(s.Breaks)-1].Direction != direction {
		kind = "CHoCH"
	}
	s.Breaks = append(s.Breaks, Break{Kind: kind, Direction: direction, Price: price, At: c.T, Index: i})
}

func (s *Structure) addOrderBlock(candles []Candle, breakIndex int, direction string) {
	for j := breakIndex - 1; j >= 0 && j >= breakIndex-10; j-- {
		c := candles[j]
		opposite := (direction == "bullish" && c.C < c.O) || (direction == "bearish" && c.C > c.O)
		if opposite {
			s.Zones = append(s.Zones, Zone{Kind: "order_block", Direction: direction,
				Top: c.H, Bottom: c.L, From: c.T, Index: j})
			return
		}
	}
}

func zoneStatus(z Zone, candles []Candle) (string, *time.Time) {
	mid := (z.Top + z.Bottom) / 2
	status := "fresh"
	// Start after the candle(s) that formed the zone.
	start := z.Index + 1
	if z.Kind == "order_block" {
		start = z.Index + 2
	}
	for i := start; i < len(candles); i++ {
		c := candles[i]
		at := c.T
		if z.Direction == "bullish" {
			if c.C < z.Bottom {
				return "invalidated", &at
			}
			if c.L <= mid {
				status = "mitigated"
			} else if c.L <= z.Top && status == "fresh" {
				status = "tested"
			}
		} else {
			if c.C > z.Top {
				return "invalidated", &at
			}
			if c.H >= mid {
				status = "mitigated"
			} else if c.H >= z.Bottom && status == "fresh" {
				status = "tested"
			}
		}
	}
	if status == "fresh" && len(candles)-z.Index > zoneExpiry {
		return "expired", nil
	}
	return status, nil
}

// LatestZones returns the most recent zones, newest last.
func (s Structure) LatestZones() []Zone {
	if len(s.Zones) <= maxZonesOut {
		return s.Zones
	}
	return s.Zones[len(s.Zones)-maxZonesOut:]
}

// ActiveZone returns the newest fresh or tested order block in direction.
func (s Structure) ActiveZone(direction string) (Zone, bool) {
	for i := len(s.Zones) - 1; i >= 0; i-- {
		z := s.Zones[i]
		if z.Kind == "order_block" && z.Direction == direction && (z.Status == "fresh" || z.Status == "tested") {
			return z, true
		}
	}
	return Zone{}, false
}
