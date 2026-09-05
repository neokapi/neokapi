package openxml

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Serial dates (ECMA-376 Part 1 §18.17.4.1). A date-time is a count of days
// from the workbook's epoch, the fraction being the time of day. The 1900
// system counts from 1899-12-31 with serial 1 as 1900-01-01 and, for
// compatibility with the original spreadsheet programs, treats 1900 as a leap
// year: serial 60 is the nonexistent 1900-02-29, and every later serial is one
// day behind a real count from 1899-12-30. The 1904 system counts from
// 1904-01-01 as serial 0 with no such quirk. The weekday follows the sheet's
// own arithmetic, so serial 1 is a Sunday in the 1900 system as it is there.

const (
	maxSerial1900 = 2958465 // 9999-12-31
	maxSerial1904 = 2957003 // 9999-12-31
	secondsPerDay = 24 * 60 * 60
)

// serialDate resolves a whole day count to its calendar date and weekday
// (0 = Sunday). The error reports a day count outside the sheet's range.
func serialDate(days int, date1904 bool) (year int, month time.Month, day, weekday int, err error) {
	if date1904 {
		if days < 0 || days > maxSerial1904 {
			return 0, 0, 0, 0, unsupportedf("serial day %d outside the 1904 range", days)
		}
		t := time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		y, m, d := t.Date()
		return y, m, d, int(t.Weekday()), nil
	}
	if days < 0 || days > maxSerial1900 {
		return 0, 0, 0, 0, unsupportedf("serial day %d outside the 1900 range", days)
	}
	weekday = (days + 6) % 7
	switch {
	case days == 0:
		// The day before 1900-01-01, which the sheet shows as 1900-01-00.
		return 1900, time.January, 0, weekday, nil
	case days < 60:
		t := time.Date(1899, time.December, 31, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		y, m, d := t.Date()
		return y, m, d, weekday, nil
	case days == 60:
		return 1900, time.February, 29, weekday, nil
	default:
		t := time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		y, m, d := t.Date()
		return y, m, d, weekday, nil
	}
}

// renderDate renders a serial value through a date section. The time is
// rounded to the finest unit the section shows (fractional seconds, seconds,
// minutes, hours) and carries into the day; a date-only section shows the day
// the serial falls in.
func (s *fmtSection) renderDate(serial float64, date1904 bool) (string, error) {
	if serial < 0 {
		return "", unsupportedf("negative serial %v", serial)
	}
	total := serial * secondsPerDay
	switch {
	case s.dateOnly:
		total = math.Floor(serial) * secondsPerDay
	case s.secFrac > 0:
		unit := math.Pow(10, -float64(s.secFrac))
		total = math.Round(total/unit) * unit
	default:
		unit := float64(s.roundUnit)
		total = math.Round(total/unit) * unit
	}
	days := math.Floor(total / secondsPerDay)
	secOfDay := total - days*secondsPerDay
	hour := int(secOfDay / 3600)
	minute := int(secOfDay/60) % 60
	second := secOfDay - float64(hour*3600) - float64(minute*60)
	if second < 0 {
		second = 0
	}

	year, month, day, weekday, err := serialDate(int(days), date1904)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tokLiteral:
			b.WriteString(t.text)
		case tokAMPM:
			b.WriteString(ampmText(t.text, hour))
		case tokElapsed:
			switch t.field {
			case fieldHour:
				b.WriteString(padInt(int(days)*24+hour, t.n))
			case fieldMinute:
				b.WriteString(padInt(int(math.Floor(total/60)), t.n))
			default:
				b.WriteString(padInt(int(math.Floor(total)), t.n))
			}
		case tokDate:
			switch t.field {
			case fieldYear:
				if t.n <= 2 {
					b.WriteString(fmt.Sprintf("%02d", year%100))
				} else {
					b.WriteString(fmt.Sprintf("%04d", year))
				}
			case fieldMonth:
				switch {
				case t.n == 1:
					b.WriteString(strconv.Itoa(int(month)))
				case t.n == 2:
					b.WriteString(fmt.Sprintf("%02d", int(month)))
				case t.n == 3:
					b.WriteString(month.String()[:3])
				case t.n == 4:
					b.WriteString(month.String())
				default:
					b.WriteString(month.String()[:1])
				}
			case fieldDay:
				switch {
				case t.n == 1:
					b.WriteString(strconv.Itoa(day))
				case t.n == 2:
					b.WriteString(fmt.Sprintf("%02d", day))
				case t.n == 3:
					b.WriteString(time.Weekday(weekday).String()[:3])
				default:
					b.WriteString(time.Weekday(weekday).String())
				}
			case fieldHour:
				h := hour
				if s.hasAMPM {
					h = hour % 12
					if h == 0 {
						h = 12
					}
				}
				b.WriteString(padInt(h, t.n))
			case fieldMinute:
				b.WriteString(padInt(minute, t.n))
			case fieldSecond:
				b.WriteString(padInt(int(math.Floor(second)), t.n))
			case fieldSecFrac:
				frac := second - math.Floor(second)
				digits := strconv.FormatFloat(frac, 'f', t.n, 64)
				if !strings.HasPrefix(digits, "0.") {
					// Float noise at the rounding boundary: the unit was
					// already rounded, so the fraction is zero.
					digits = "0." + strings.Repeat("0", t.n)
				}
				b.WriteString(digits[1:])
			}
		}
	}
	return b.String(), nil
}

// ampmText renders the 12-hour marker in the case and length it was written:
// AM/PM, am/pm, A/P or a/p.
func ampmText(marker string, hour int) string {
	am, pm, _ := strings.Cut(marker, "/")
	if hour < 12 {
		return am
	}
	return pm
}

// padInt renders n with at least two digits when the field letter was doubled.
func padInt(n, width int) string {
	if width >= 2 {
		return fmt.Sprintf("%02d", n)
	}
	return strconv.Itoa(n)
}
