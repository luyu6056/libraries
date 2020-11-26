package libraries

import "time"

//忽然发现，每年的节假日都会变动的，无法预测
//白名单
var holiday = map[string]bool{
	"2020-04-06": true,
	"2020-05-04": true,
	"2020-05-05": true,
	"2020-06-25": true,
	"2020-06-26": true,
	"2020-06-27": true,
	"2020-10-04": true,
	"2020-10-05": true,
	"2020-10-06": true,
	"2020-10-07": true,
	"2020-10-08": true,
}

//黑名单，加班日期
var work_overtime = map[string]bool{
	"2020-04-26": true,
	"2020-05-09": true,
	"2020-06-28": true,
	"2020-09-27": true,
	"2020-10-10": true,
}

func IsHoliday(t time.Time) bool {
	day := t.Format("2006-01-02")
	if _, ok := work_overtime[day]; ok {
		return false
	}
	if _, ok := holiday[day]; ok {
		return true
	}
	switch t.Month().String() {
	case "January":
		//元旦
		if t.Day() == 1 {
			return true
		}
	case "May", "October":
		//51,国庆
		if t.Day() == 1 || t.Day() == 2 || t.Day() == 3 {
			return true
		}
	}
	return t.Weekday().String() == "Saturday" || t.Weekday().String() == "Sunday"
}
