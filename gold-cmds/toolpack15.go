package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 15 (10 new everyday commands)
// File: toolpack15.go
// ============================================================================
//   .prayertimes <city> <country> -> daily prayer (namaz) times
//   .hijridate [dd-mm-yyyy]       -> gregorian to hijri date
//   .qibladirection <lat> <lon>   -> qibla direction in degrees
//   .asmaulhusna [number]         -> 99 names of Allah
//   .worldclock <timezone>        -> current time in any timezone
//   .timezoneconvert <from> <to> <datetime> -> convert time between zones
//   .ipdetails <ip>               -> full geolocation of an IP
//   .countryfacts <country>       -> capital, currency, population
//   .countrycapital <country>     -> capital city of a country
//   .sslcheck <host>              -> SSL/TLS grade of a website
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .PRAYERTIMES ─────────────────────────────────────────────────────────────

func prayertimesGuide(prefix string) string {
	return "*🔰 PRAYER TIMES 🔰*\n\n" +
		"*GET TODAY'S NAMAZ (SALAH) TIMES FOR ANY CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PRAYERTIMES <CITY> <COUNTRY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PRAYERTIMES KARACHI PAKISTAN ❯*"
}

func handlePrayertimes(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, prayertimesGuide(prefix))
			return
		}
		city := strings.TrimSpace(args[0])
		country := strings.TrimSpace(strings.Join(args[1:], " "))
		waitID := s.ReplyWithID(info, "*FETCHING PRAYER TIMES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data struct {
				Timings map[string]string `json:"timings"`
				Date    struct {
					Readable string `json:"readable"`
					Hijri    struct {
						Date string `json:"date"`
					} `json:"hijri"`
				} `json:"date"`
			} `json:"data"`
		}
		u := "https://api.aladhan.com/v1/timingsByCity?city=" + url.QueryEscape(city) + "&country=" + url.QueryEscape(country) + "&method=1"
		if err := funGetJSON(ctx, u, &res); err != nil || res.Data.Timings == nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "PRAYER TIMES")
			}
			return
		}
		t := res.Data.Timings
		var b strings.Builder
		b.WriteString("*🔰 PRAYER TIMES 🔰*\n\n")
		b.WriteString("*📍 " + strings.ToUpper(city) + ", " + strings.ToUpper(country) + "*\n")
		b.WriteString("*📅 " + strings.ToUpper(res.Data.Date.Readable) + "*\n\n")
		b.WriteString("*🌅 FAJR ❯ " + t["Fajr"] + "*\n")
		b.WriteString("*☀️ SUNRISE ❯ " + t["Sunrise"] + "*\n")
		b.WriteString("*🕛 DHUHR ❯ " + t["Dhuhr"] + "*\n")
		b.WriteString("*🌤️ ASR ❯ " + t["Asr"] + "*\n")
		b.WriteString("*🌇 MAGHRIB ❯ " + t["Maghrib"] + "*\n")
		b.WriteString("*🌙 ISHA ❯ " + t["Isha"] + "*")
		s.Reply(info, b.String())
	})
}

// ── .HIJRIDATE ───────────────────────────────────────────────────────────────

func hijridateGuide(prefix string) string {
	return "*🔰 HIJRI DATE 🔰*\n\n" +
		"*CONVERT A GREGORIAN DATE TO THE HIJRI (ISLAMIC) DATE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "HIJRIDATE ❯*  (today)\n" +
		"*❮ " + prefix + "HIJRIDATE <DD-MM-YYYY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "HIJRIDATE 18-09-2026 ❯*"
}

func handleHijridate(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		date := ""
		if len(args) > 0 {
			date = strings.TrimSpace(args[0])
		}
		waitID := s.ReplyWithID(info, "*CONVERTING DATE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		u := "https://api.aladhan.com/v1/gToH"
		if date != "" {
			u += "?date=" + url.QueryEscape(date)
		}
		var res struct {
			Data struct {
				Hijri struct {
					Date  string `json:"date"`
					Day   string `json:"day"`
					Month struct {
						En string `json:"en"`
					} `json:"month"`
					Year string `json:"year"`
				} `json:"hijri"`
				Gregorian struct {
					Date string `json:"date"`
				} `json:"gregorian"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Data.Hijri.Date == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "HIJRI DATE")
			}
			return
		}
		h := res.Data.Hijri
		s.Reply(info, "*🔰 HIJRI DATE 🔰*\n\n*📅 GREGORIAN ❯ "+res.Data.Gregorian.Date+"*\n*🌙 HIJRI ❯ "+h.Day+" "+strings.ToUpper(h.Month.En)+" "+h.Year+" AH*\n*🗓️ FULL ❯ "+h.Date+"*")
	})
}

// ── .QIBLADIRECTION ──────────────────────────────────────────────────────────

func qibladirectionGuide(prefix string) string {
	return "*🔰 QIBLA DIRECTION 🔰*\n\n" +
		"*FIND THE QIBLA DIRECTION FROM ANY LOCATION*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "QIBLADIRECTION <LATITUDE> <LONGITUDE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "QIBLADIRECTION 24.86 67.01 ❯*"
}

func handleQibladirection(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 2 {
			s.Reply(info, qibladirectionGuide(prefix))
			return
		}
		lat := strings.TrimSpace(args[0])
		lon := strings.TrimSpace(args[1])
		waitID := s.ReplyWithID(info, "*CALCULATING QIBLA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Direction float64 `json:"direction"`
			} `json:"data"`
		}
		u := "https://api.aladhan.com/v1/qibla/" + url.QueryEscape(lat) + "/" + url.QueryEscape(lon)
		if err := funGetJSON(ctx, u, &res); err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "QIBLA DIRECTION")
			}
			return
		}
		s.Reply(info, "*🔰 QIBLA DIRECTION 🔰*\n\n*📍 LOCATION ❯ "+strconv.FormatFloat(res.Data.Latitude, 'f', 4, 64)+", "+strconv.FormatFloat(res.Data.Longitude, 'f', 4, 64)+"*\n*🕋 DIRECTION ❯ "+strconv.FormatFloat(res.Data.Direction, 'f', 2, 64)+"° FROM NORTH*")
	})
}

// ── .ASMAULHUSNA ─────────────────────────────────────────────────────────────

func asmaulhusnaGuide(prefix string) string {
	return "*🔰 ASMA UL HUSNA 🔰*\n\n" +
		"*THE 99 BEAUTIFUL NAMES OF ALLAH*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ASMAULHUSNA <NUMBER 1-99> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ASMAULHUSNA 1 ❯*"
}

func handleAsmaulhusna(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) == 0 {
			s.Reply(info, asmaulhusnaGuide(prefix))
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || n < 1 || n > 99 {
			s.Reply(info, "*🔰 PLEASE GIVE A NUMBER BETWEEN 1 AND 99*")
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING NAME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data []struct {
				Name            string `json:"name"`
				Transliteration string `json:"transliteration"`
				Number          int    `json:"number"`
				En              struct {
					Meaning string `json:"meaning"`
				} `json:"en"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://api.aladhan.com/v1/asmaAlHusna", &res); err != nil || len(res.Data) < n {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "ASMA UL HUSNA")
			}
			return
		}
		a := res.Data[n-1]
		s.Reply(info, "*🔰 ASMA UL HUSNA 🔰*\n\n*🔢 NUMBER ❯ "+strconv.Itoa(a.Number)+"*\n*🕌 ARABIC ❯ "+a.Name+"*\n*📝 TRANSLITERATION ❯ "+strings.ToUpper(a.Transliteration)+"*\n*💡 MEANING ❯ "+strings.ToUpper(a.En.Meaning)+"*")
	})
}

// ── .WORLDCLOCK ──────────────────────────────────────────────────────────────

func worldclockGuide(prefix string) string {
	return "*🔰 WORLD CLOCK 🔰*\n\n" +
		"*GET THE CURRENT TIME IN ANY TIMEZONE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WORLDCLOCK <TIMEZONE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WORLDCLOCK ASIA/KARACHI ❯*"
}

func handleWorldclock(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		tz := strings.TrimSpace(strings.Join(args, " "))
		if tz == "" {
			s.Reply(info, worldclockGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CHECKING TIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			DateTime  string `json:"dateTime"`
			Date      string `json:"date"`
			Time      string `json:"time"`
			TimeZone  string `json:"timeZone"`
			DayOfWeek string `json:"dayOfWeek"`
		}
		u := "https://timeapi.io/api/Time/current/zone?timeZone=" + url.QueryEscape(tz)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Time == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "WORLD CLOCK")
			}
			return
		}
		s.Reply(info, "*🔰 WORLD CLOCK 🔰*\n\n*🌍 ZONE ❯ "+strings.ToUpper(res.TimeZone)+"*\n*📅 DATE ❯ "+res.Date+"*\n*🕒 TIME ❯ "+res.Time+"*\n*📆 DAY ❯ "+strings.ToUpper(res.DayOfWeek)+"*")
	})
}

// ── .TIMEZONECONVERT ─────────────────────────────────────────────────────────

func timezoneconvertGuide(prefix string) string {
	return "*🔰 TIMEZONE CONVERTER 🔰*\n\n" +
		"*CONVERT A DATE AND TIME BETWEEN TWO TIMEZONES*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIMEZONECONVERT <FROM> <TO> <YYYY-MM-DD HH:MM> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIMEZONECONVERT AMERICA/NEW_YORK ASIA/KARACHI 2026-09-18 12:00 ❯*"
}

func handleTimezoneconvert(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 4 {
			s.Reply(info, timezoneconvertGuide(prefix))
			return
		}
		from := strings.TrimSpace(args[0])
		to := strings.TrimSpace(args[1])
		dt := strings.TrimSpace(args[2]) + " " + strings.TrimSpace(args[3])
		// timeapi.io requires seconds in the datetime (HH:MM:SS).
		if parts := strings.Split(dt, " "); len(parts) == 2 && strings.Count(parts[1], ":") == 1 {
			dt = parts[0] + " " + parts[1] + ":00"
		}
		waitID := s.ReplyWithID(info, "*CONVERTING TIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		body := []byte(`{"fromTimeZone":"` + from + `","dateTime":"` + dt + `","toTimeZone":"` + to + `","dstAmbiguity":""}`)
		var res struct {
			FromDateTime     string `json:"fromDateTime"`
			ToTimeZone       string `json:"toTimeZone"`
			ConversionResult struct {
				DateTime string `json:"dateTime"`
				Time     string `json:"time"`
			} `json:"conversionResult"`
		}
		if err := funPostJSON(ctx, "https://timeapi.io/api/Conversion/ConvertTimeZone", body, &res); err != nil || res.ConversionResult.DateTime == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "TIMEZONE CONVERTER")
			}
			return
		}
		s.Reply(info, "*🔰 TIMEZONE CONVERTER 🔰*\n\n*🕐 FROM ❯ "+strings.ToUpper(from)+" "+res.FromDateTime+"*\n*🕓 TO ❯ "+strings.ToUpper(res.ToTimeZone)+" "+res.ConversionResult.DateTime+"*")
	})
}

// ── .IPDETAILS ───────────────────────────────────────────────────────────────

func ipdetailsGuide(prefix string) string {
	return "*🔰 IP DETAILS 🔰*\n\n" +
		"*GET FULL GEOLOCATION AND ISP OF ANY IP ADDRESS*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "IPDETAILS <IP> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "IPDETAILS 8.8.8.8 ❯*"
}

func handleIpdetails(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		ip := strings.TrimSpace(args[0])
		if ip == "" {
			s.Reply(info, ipdetailsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*LOOKING UP IP....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			IP          string  `json:"ip"`
			Success     bool    `json:"success"`
			Type        string  `json:"type"`
			Country     string  `json:"country"`
			Region      string  `json:"region"`
			City        string  `json:"city"`
			Postal      string  `json:"postal"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			CallingCode string  `json:"calling_code"`
			Connection  struct {
				ISP string `json:"isp"`
				Org string `json:"org"`
			} `json:"connection"`
			Timezone struct {
				ID string `json:"id"`
			} `json:"timezone"`
		}
		if err := funGetJSON(ctx, "https://ipwho.is/"+url.QueryEscape(ip), &res); err != nil || !res.Success {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "IP DETAILS")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 IP DETAILS 🔰*\n\n")
		b.WriteString("*🌐 IP ❯ " + res.IP + " (" + strings.ToUpper(res.Type) + ")*\n")
		b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(res.Country) + "*\n")
		b.WriteString("*🏙️ CITY ❯ " + strings.ToUpper(res.City) + ", " + strings.ToUpper(res.Region) + "*\n")
		b.WriteString("*📮 POSTAL ❯ " + res.Postal + "*\n")
		b.WriteString("*📍 COORDS ❯ " + strconv.FormatFloat(res.Latitude, 'f', 4, 64) + ", " + strconv.FormatFloat(res.Longitude, 'f', 4, 64) + "*\n")
		b.WriteString("*🏢 ISP ❯ " + strings.ToUpper(res.Connection.ISP) + "*\n")
		b.WriteString("*🕒 TIMEZONE ❯ " + res.Timezone.ID + "*")
		s.Reply(info, b.String())
	})
}

// ── .COUNTRYFACTS ────────────────────────────────────────────────────────────

func countryfactsGuide(prefix string) string {
	return "*🔰 COUNTRY FACTS 🔰*\n\n" +
		"*GET CAPITAL, CURRENCY AND POPULATION OF ANY COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTRYFACTS <COUNTRY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COUNTRYFACTS PAKISTAN ❯*"
}

func handleCountryfacts(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		country := strings.TrimSpace(strings.Join(args, " "))
		if country == "" {
			s.Reply(info, countryfactsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING COUNTRY FACTS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var capRes struct {
			Error bool `json:"error"`
			Data  struct {
				Name    string `json:"name"`
				Capital string `json:"capital"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://countriesnow.space/api/v0.1/countries/capital/q?country="+url.QueryEscape(country), &capRes); err != nil || capRes.Error {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COUNTRY FACTS")
			}
			return
		}
		var curRes struct {
			Error bool `json:"error"`
			Data  struct {
				Currency string `json:"currency"`
			} `json:"data"`
		}
		_ = funGetJSON(ctx, "https://countriesnow.space/api/v0.1/countries/currency/q?country="+url.QueryEscape(country), &curRes)
		var popRes struct {
			Error bool `json:"error"`
			Data  struct {
				PopulationCounts []struct {
					Year  int    `json:"year"`
					Value string `json:"value"`
				} `json:"populationCounts"`
			} `json:"data"`
		}
		_ = funGetJSON(ctx, "https://countriesnow.space/api/v0.1/countries/population/q?country="+url.QueryEscape(country), &popRes)
		var b strings.Builder
		b.WriteString("*🔰 COUNTRY FACTS 🔰*\n\n")
		b.WriteString("*🌍 COUNTRY ❯ " + strings.ToUpper(capRes.Data.Name) + "*\n")
		b.WriteString("*🏛️ CAPITAL ❯ " + strings.ToUpper(capRes.Data.Capital) + "*\n")
		if curRes.Data.Currency != "" {
			b.WriteString("*💰 CURRENCY ❯ " + strings.ToUpper(curRes.Data.Currency) + "*\n")
		}
		if n := len(popRes.Data.PopulationCounts); n > 0 {
			last := popRes.Data.PopulationCounts[n-1]
			b.WriteString("*👥 POPULATION ❯ " + last.Value + " (" + strconv.Itoa(last.Year) + ")*")
		} else {
			b.WriteString("*👥 POPULATION ❯ N/A*")
		}
		s.Reply(info, b.String())
	})
}

// ── .COUNTRYCAPITAL ──────────────────────────────────────────────────────────

func countrycapitalGuide(prefix string) string {
	return "*🔰 COUNTRY CAPITAL 🔰*\n\n" +
		"*FIND THE CAPITAL CITY OF ANY COUNTRY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "COUNTRYCAPITAL <COUNTRY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "COUNTRYCAPITAL JAPAN ❯*"
}

func handleCountrycapital(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		country := strings.TrimSpace(strings.Join(args, " "))
		if country == "" {
			s.Reply(info, countrycapitalGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FINDING CAPITAL....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Error bool `json:"error"`
			Data  struct {
				Name    string `json:"name"`
				Capital string `json:"capital"`
				Iso2    string `json:"iso2"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, "https://countriesnow.space/api/v0.1/countries/capital/q?country="+url.QueryEscape(country), &res); err != nil || res.Error {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "COUNTRY CAPITAL")
			}
			return
		}
		s.Reply(info, "*🔰 COUNTRY CAPITAL 🔰*\n\n*🌍 COUNTRY ❯ "+strings.ToUpper(res.Data.Name)+"*\n*🏛️ CAPITAL ❯ "+strings.ToUpper(res.Data.Capital)+"*\n*🔤 ISO ❯ "+strings.ToUpper(res.Data.Iso2)+"*")
	})
}

// ── .SSLCHECK ────────────────────────────────────────────────────────────────

func sslcheckGuide(prefix string) string {
	return "*🔰 SSL CHECKER 🔰*\n\n" +
		"*CHECK THE SSL/TLS SECURITY GRADE OF ANY WEBSITE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SSLCHECK <HOST> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SSLCHECK GITHUB.COM ❯*"
}

func handleSslcheck(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		host := strings.TrimSpace(args[0])
		if host == "" {
			s.Reply(info, sslcheckGuide(prefix))
			return
		}
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimSuffix(host, "/")
		waitID := s.ReplyWithID(info, "*ANALYZING SSL....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Host      string `json:"host"`
			Status    string `json:"status"`
			Endpoints []struct {
				IPAddress     string `json:"ipAddress"`
				Grade         string `json:"grade"`
				StatusMessage string `json:"statusMessage"`
			} `json:"endpoints"`
		}
		u := "https://api.ssllabs.com/api/v3/analyze?host=" + url.QueryEscape(host) + "&publish=off&all=done"
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Endpoints) == 0 {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SSL CHECKER")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 SSL CHECKER 🔰*\n\n")
		b.WriteString("*🌐 HOST ❯ " + strings.ToUpper(res.Host) + "*\n")
		b.WriteString("*📊 STATUS ❯ " + strings.ToUpper(res.Status) + "*\n\n")
		for _, e := range res.Endpoints {
			grade := e.Grade
			if grade == "" {
				grade = "N/A"
			}
			b.WriteString("*🖥️ " + e.IPAddress + " ❯ GRADE " + strings.ToUpper(grade) + "*\n")
		}
		s.Reply(info, strings.TrimRight(b.String(), "\n"))
	})
}

func init() {
	Register(Command{Name: "prayertimes", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TODAY'S NAMAZ TIMES FOR ANY CITY. USE IT AS .PRAYERTIMES <CITY> <COUNTRY>.", Run: handlePrayertimes})
	Register(Command{Name: "hijridate", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT A GREGORIAN DATE TO THE HIJRI DATE. USE IT AS .HIJRIDATE <DD-MM-YYYY>.", Run: handleHijridate})
	Register(Command{Name: "qibladirection", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE QIBLA DIRECTION FROM ANY LOCATION. USE IT AS .QIBLADIRECTION <LAT> <LON>.", Run: handleQibladirection})
	Register(Command{Name: "asmaulhusna", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO SHOW THE 99 NAMES OF ALLAH BY NUMBER. USE IT AS .ASMAULHUSNA <NUMBER>.", Run: handleAsmaulhusna})
	Register(Command{Name: "worldclock", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE CURRENT TIME IN ANY TIMEZONE. USE IT AS .WORLDCLOCK <TIMEZONE>.", Run: handleWorldclock})
	Register(Command{Name: "timezoneconvert", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT TIME BETWEEN TWO TIMEZONES. USE IT AS .TIMEZONECONVERT <FROM> <TO> <DATETIME>.", Run: handleTimezoneconvert})
	Register(Command{Name: "ipdetails", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET FULL GEOLOCATION AND ISP OF ANY IP. USE IT AS .IPDETAILS <IP>.", Run: handleIpdetails})
	Register(Command{Name: "countryfacts", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET CAPITAL, CURRENCY AND POPULATION OF A COUNTRY. USE IT AS .COUNTRYFACTS <COUNTRY>.", Run: handleCountryfacts})
	Register(Command{Name: "countrycapital", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO FIND THE CAPITAL CITY OF ANY COUNTRY. USE IT AS .COUNTRYCAPITAL <COUNTRY>.", Run: handleCountrycapital})
	Register(Command{Name: "sslcheck", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CHECK THE SSL/TLS GRADE OF ANY WEBSITE. USE IT AS .SSLCHECK <HOST>.", Run: handleSslcheck})

	// hidden aliases
	Register(Command{Name: "namaztime", Category: "TOOLS", Desc: "Short alias of .prayertimes", Hidden: true, Run: handlePrayertimes})
	Register(Command{Name: "hijri", Category: "TOOLS", Desc: "Short alias of .hijridate", Hidden: true, Run: handleHijridate})
	Register(Command{Name: "qibla", Category: "TOOLS", Desc: "Short alias of .qibladirection", Hidden: true, Run: handleQibladirection})
	Register(Command{Name: "namesofallah", Category: "TOOLS", Desc: "Short alias of .asmaulhusna", Hidden: true, Run: handleAsmaulhusna})
	Register(Command{Name: "clock", Category: "TOOLS", Desc: "Short alias of .worldclock", Hidden: true, Run: handleWorldclock})
	Register(Command{Name: "tzconvert", Category: "TOOLS", Desc: "Short alias of .timezoneconvert", Hidden: true, Run: handleTimezoneconvert})
	Register(Command{Name: "iplookup", Category: "TOOLS", Desc: "Short alias of .ipdetails", Hidden: true, Run: handleIpdetails})
	Register(Command{Name: "countryinfo", Category: "TOOLS", Desc: "Short alias of .countryfacts", Hidden: true, Run: handleCountryfacts})
	Register(Command{Name: "capital", Category: "TOOLS", Desc: "Short alias of .countrycapital", Hidden: true, Run: handleCountrycapital})
	Register(Command{Name: "ssltest", Category: "TOOLS", Desc: "Short alias of .sslcheck", Hidden: true, Run: handleSslcheck})
}
