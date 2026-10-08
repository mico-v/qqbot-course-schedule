package schedule

import "time"

// AcademicYearStartMonth is the first month of an academic year. A new
// academic year begins on Aug 1, so every date from Aug 1 through Jul 31
// belongs to the academic year that started that August. The whole system
// resolves year-less dates inside this window and keeps 调休 shifts within one
// academic year.
const AcademicYearStartMonth = time.August

// AcademicYearStart returns Aug 1 of the academic year that contains day.
func AcademicYearStart(day time.Time) time.Time {
	local := day.In(LocalTZ)
	year := local.Year()
	if local.Month() < AcademicYearStartMonth {
		year--
	}
	return time.Date(year, AcademicYearStartMonth, 1, 0, 0, 0, 0, LocalTZ)
}

// AcademicYearEnd returns the exclusive end (Aug 1) of day's academic year.
func AcademicYearEnd(day time.Time) time.Time {
	return AcademicYearStart(day).AddDate(1, 0, 0)
}

// SameAcademicYear reports whether a and b fall in the same academic year.
func SameAcademicYear(a, b time.Time) bool {
	return AcademicYearStart(a).Equal(AcademicYearStart(b))
}

// AcademicYearLabel renders the academic year of day as "2026-2027".
func AcademicYearLabel(day time.Time) string {
	start := AcademicYearStart(day)
	return start.Format("2006") + "-" + start.AddDate(1, 0, 0).Format("2006")
}

// resolveAcademicYear picks the year for a month/day so the result lands in
// ref's academic year. A month on or after August uses the start year, a month
// before August uses the following calendar year.
func resolveAcademicYear(month time.Month, day int, ref time.Time) (time.Time, bool) {
	year := AcademicYearStart(ref).Year()
	if month < AcademicYearStartMonth {
		year++
	}
	return dayAt(year, month, day)
}
