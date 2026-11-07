package i18n

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// style is how a language writes numbers, prices and dates, after CLDR.
type style struct {
	decimal, group string
	// minGroup is how many digits must stand before the first group
	// separator: Spanish writes 1234 but 12.345 (CLDR minimumGroupingDigits).
	minGroup int
	// symbolAfter puts the currency after the number: "12,50 €".
	symbolAfter bool
	// symbolSpace goes between a currency symbol and the number, codeSpace
	// between a currency code and the number ("SEK 12.50").
	symbolSpace, codeSpace string
	months                 [12]string // abbreviated, as dates use them
	// day, date and dayTime are patterns: {d} the day of the month, {m} the
	// month, {y} the year, {t} the time as 15:04.
	day, date, dayTime string
}

const nbsp = " "

var styles = map[string]style{
	// English keeps the site's first format, without a group separator.
	"en": {decimal: ".", codeSpace: " ",
		months: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		day:    "{m} {d}", date: "{m} {d}, {y}", dayTime: "{m} {d}, {t}"},
	"de": {decimal: ",", group: ".", minGroup: 1, symbolAfter: true, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"Jan.", "Feb.", "März", "Apr.", "Mai", "Juni", "Juli", "Aug.", "Sept.", "Okt.", "Nov.", "Dez."},
		day:    "{d}. {m}", date: "{d}. {m} {y}", dayTime: "{d}. {m}, {t}"},
	"fr": {decimal: ",", group: " ", minGroup: 1, symbolAfter: true, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
		day:    "{d} {m}", date: "{d} {m} {y}", dayTime: "{d} {m}, {t}"},
	"es": {decimal: ",", group: ".", minGroup: 2, symbolAfter: true, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sept", "oct", "nov", "dic"},
		day:    "{d} {m}", date: "{d} {m} {y}", dayTime: "{d} {m}, {t}"},
	"it": {decimal: ",", group: ".", minGroup: 1, symbolAfter: true, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"gen", "feb", "mar", "apr", "mag", "giu", "lug", "ago", "set", "ott", "nov", "dic"},
		day:    "{d} {m}", date: "{d} {m} {y}", dayTime: "{d} {m}, {t}"},
	// Portuguese as written in Portugal, the EU market's.
	"pt": {decimal: ",", group: nbsp, minGroup: 2, symbolAfter: true, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"jan.", "fev.", "mar.", "abr.", "mai.", "jun.", "jul.", "ago.", "set.", "out.", "nov.", "dez."},
		day:    "{d} de {m}", date: "{d} de {m} de {y}", dayTime: "{d} de {m}, {t}"},
	"nl": {decimal: ",", group: ".", minGroup: 1, symbolSpace: nbsp, codeSpace: nbsp,
		months: [12]string{"jan", "feb", "mrt", "apr", "mei", "jun", "jul", "aug", "sep", "okt", "nov", "dec"},
		day:    "{d} {m}", date: "{d} {m} {y}", dayTime: "{d} {m}, {t}"},
	"tr": {decimal: ",", group: ".", minGroup: 1, codeSpace: nbsp,
		months: [12]string{"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"},
		day:    "{d} {m}", date: "{d} {m} {y}", dayTime: "{d} {m} {t}"},
}

func styleOf(lang string) style {
	if st, ok := styles[lang]; ok {
		return st
	}
	return styles[Source]
}

// Number writes a whole number: 12345 is "12345" in English (the site's
// first format) and "12.345" in German.
func Number(lang string, n int64) string {
	return styleOf(lang).number(n)
}

func (st style) number(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, digits = "-", digits[1:]
	}
	if st.group == "" || len(digits) < 3+st.minGroup {
		return sign + digits
	}
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	var b strings.Builder
	b.WriteString(sign + digits[:head])
	for i := head; i < len(digits); i += 3 {
		b.WriteString(st.group + digits[i:i+3])
	}
	return b.String()
}

var symbols = map[string]string{"USD": "$", "GBP": "£", "EUR": "€", "TRY": "₺"}

// Money writes minor units of a currency: 1925 USD is "$19.25" in English,
// "19,25 $" in German. A currency without a symbol here shows its code.
// Prices are never negative; a caller showing a difference writes the sign.
func Money(lang string, minor int64, currency string) string {
	st := styleOf(lang)
	num := st.number(minor/100) + st.decimal + fmt.Sprintf("%02d", minor%100)
	sym, space := symbols[currency], st.symbolSpace
	if sym == "" {
		sym, space = currency, st.codeSpace
	}
	if st.symbolAfter {
		return num + space + sym
	}
	return sym + space + num
}

// Day is a date without its year ("Nov 2"), Date one with it ("Nov 2,
// 2026") and DayTime a day with its time ("Nov 2, 15:04"). All read the time
// in UTC, as the site's readings are.
func Day(lang string, t time.Time) string {
	st := styleOf(lang)
	return st.when(st.day, t)
}

// Date: see Day.
func Date(lang string, t time.Time) string {
	st := styleOf(lang)
	return st.when(st.date, t)
}

// DayTime: see Day.
func DayTime(lang string, t time.Time) string {
	st := styleOf(lang)
	return st.when(st.dayTime, t)
}

func (st style) when(pattern string, t time.Time) string {
	t = t.UTC()
	return strings.NewReplacer("{d}", strconv.Itoa(t.Day()), "{m}", st.months[t.Month()-1],
		"{y}", strconv.Itoa(t.Year()), "{t}", t.Format("15:04")).Replace(pattern)
}
