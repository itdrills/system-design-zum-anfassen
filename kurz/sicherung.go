package main

import (
	"log"
	"net/http"
	"sync"
	"time"
)

// sicherung ist ein einfacher Circuit Breaker: Nach `schwelle` Serverfehlern in Folge
// lehnt er `pause` lang alles sofort ab (offen). Danach lässt er eine einzige Probe
// durch (halb offen). Gelingt sie, ist er wieder geschlossen, sonst wieder offen.
type sicherung struct {
	mu       sync.Mutex
	schwelle int
	pause    time.Duration
	fehler   int       // Serverfehler in Folge
	offenBis time.Time // bis dahin sofort ablehnen
	probe    bool      // halb offen: eine Probe ist unterwegs
}

var schreiben = &sicherung{schwelle: 5, pause: 5 * time.Second}

func (s *sicherung) erlaubt() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.offenBis) {
		return false
	}
	if s.fehler >= s.schwelle { // halb offen
		if s.probe {
			return false
		}
		s.probe = true
	}
	return true
}

func (s *sicherung) melde(serverfehler bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probe = false
	if !serverfehler {
		if s.fehler >= s.schwelle {
			log.Printf("%s: Sicherung geschlossen", instanz)
		}
		s.fehler = 0
		return
	}
	s.fehler++
	if s.fehler >= s.schwelle {
		s.offenBis = time.Now().Add(s.pause)
		log.Printf("%s: Sicherung offen für %s (%d Fehler in Folge)",
			instanz, s.pause, s.fehler)
	}
}

// statusMerker merkt sich den Status, den ein Handler sendet.
type statusMerker struct {
	http.ResponseWriter
	status int
}

func (m *statusMerker) WriteHeader(status int) {
	m.status = status
	m.ResponseWriter.WriteHeader(status)
}

// gesichert: Ist die Sicherung offen, antwortet kurz sofort mit 503.
func gesichert(s *sicherung, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.erlaubt() {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Datenbank gestört", http.StatusServiceUnavailable)
			return
		}
		m := &statusMerker{ResponseWriter: w, status: http.StatusOK}
		next(m, r)
		s.melde(m.status >= 500)
	}
}
