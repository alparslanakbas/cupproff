package shopify

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// A bag size as the store writes it: "12 oz", "340g", "2 lb", "1 kg",
// "1/4 Pound" (Rishi), optionally with a pack count in front ("2 x 12 oz").
// Without the fraction form "1/4 Pound" read as "4 Pound", 16 times the bag.
// A fraction is a real one only (one-digit numerator, denominator 2, 3, 4, 8
// or 16): 19grams' "600/750 g" once read as 600/750 = 0.8 g.
//
// Fluid ounces are excluded on purpose: "32 fl oz" is a volume (cold brew
// concentrate), and turning it into grams would invent data.
var weightPattern = regexp.MustCompile(
	`(?i)(?:(\d+)\s*[x×]\s*)?([1-9]\s*/\s*(?:16|2|3|4|8)|\d+(?:[.,]\d+)?)\s*(fl\.?\s*oz|oz|ounces?|lbs?|pounds?|kg|kilos?|kilograms?|g|grams?|gr)\b`)

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
		amount, text, err := parseAmount(m[2])
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
		label = text + " " + unitLabel[unit]
		if count > 1 {
			label = m[1] + " x " + label
		}
		return g, label
	}
	return 0, ""
}

// parseAmount reads "12", "12.5", "12,5" or "1/4", and returns the number
// with the way the label writes it ("1/4" stays a fraction).
func parseAmount(s string) (float64, string, error) {
	if num, den, ok := strings.Cut(s, "/"); ok {
		n, err1 := strconv.Atoi(strings.TrimSpace(num))
		d, err2 := strconv.Atoi(strings.TrimSpace(den))
		if err1 != nil || err2 != nil || d == 0 {
			return 0, "", fmt.Errorf("bad fraction %q", s)
		}
		return float64(n) / float64(d), fmt.Sprintf("%d/%d", n, d), nil
	}
	f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	return f, strconv.FormatFloat(f, 'f', -1, 64), err
}

// Labels are written one way whatever the store typed ("12oz" → "12 oz").
var unitLabel = map[string]string{
	"oz": "oz", "ounce": "oz", "ounces": "oz",
	"lb": "lb", "lbs": "lb", "pound": "lb", "pounds": "lb",
	"kg": "kg", "kilo": "kg", "kilos": "kg", "kilogram": "kg", "kilograms": "kg",
	"g": "g", "gr": "g", "gram": "g", "grams": "g",
}
