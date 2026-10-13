package shopify

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// A bag size as the store writes it: "12 oz", "340g", "2 lb", "1 kg",
// optionally with a pack count in front ("2 x 12 oz").
//
// Fluid ounces are excluded on purpose: "32 fl oz" is a volume (cold brew
// concentrate), and turning it into grams would invent data.
var weightPattern = regexp.MustCompile(
	`(?i)(?:(\d+)\s*[x×]\s*)?(\d+(?:[.,]\d+)?)\s*(fl\.?\s*oz|oz|ounces?|lbs?|pounds?|kg|kilos?|kilograms?|g|grams?|gr)\b`)

var gramsPerUnit = map[string]float64{
	"oz": 28.349523125, "ounce": 28.349523125, "ounces": 28.349523125,
	"lb": 453.59237, "lbs": 453.59237, "pound": 453.59237, "pounds": 453.59237,
	"kg": 1000, "kilo": 1000, "kilos": 1000, "kilogram": 1000, "kilograms": 1000,
	"g": 1, "gr": 1, "gram": 1, "grams": 1,
}

// ParseWeight returns the net weight in grams stated in s, and a uniform
// size label ("12 oz", "2 x 12 oz"). It returns 0 when s states no weight; the caller
// must then leave the weight empty rather than guess one.
func ParseWeight(s string) (grams int, label string) {
	for _, m := range weightPattern.FindAllStringSubmatch(s, -1) {
		unit := strings.ToLower(m[3])
		if strings.HasPrefix(unit, "fl") {
			continue
		}
		amount, err := strconv.ParseFloat(strings.Replace(m[2], ",", ".", 1), 64)
		if err != nil || amount <= 0 {
			continue
		}
		count := 1.0
		if m[1] != "" {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				count = float64(n)
			}
		}
		g := int(math.Round(count * amount * gramsPerUnit[unit]))
		if g <= 0 {
			continue
		}
		label = strconv.FormatFloat(amount, 'f', -1, 64) + " " + unitLabel[unit]
		if count > 1 {
			label = m[1] + " x " + label
		}
		return g, label
	}
	return 0, ""
}

// Labels are written one way whatever the store typed ("12oz" → "12 oz").
var unitLabel = map[string]string{
	"oz": "oz", "ounce": "oz", "ounces": "oz",
	"lb": "lb", "lbs": "lb", "pound": "lb", "pounds": "lb",
	"kg": "kg", "kilo": "kg", "kilos": "kg", "kilogram": "kg", "kilograms": "kg",
	"g": "g", "gr": "g", "gram": "g", "grams": "g",
}
