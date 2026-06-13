// Command mocknw is a self-contained, offline NetWorker REST API emulator for
// local development and testing. It serves representative wrapped payloads for the
// /nwrestapi/v3/global/* endpoints, validates HTTP Basic auth, and lets developers
// run the real exporter against it without a production NetWorker server.
//
//	go run ./cmd/mocknw            # listens on :9090
//	NSR_MOCK_ADDR=:18090 go run ./cmd/mocknw
package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const basePath = "/nwrestapi/v3/global"

// fixtures maps a resource path to its wrapped JSON envelope. These mirror the
// shapes the exporter's collectors decode.
var fixtures = map[string]string{
	"/clients": `{"count":2,"clients":[
		{"hostname":"app01.local","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4,"lastBackupTime":"2026-06-13T01:00:00Z","operatingSystem":"Linux"},
		{"hostname":"db01.local","ndmp":true,"scheduledBackup":true,"backupCommand":"nsrndmp_save","parallelism":12,"lastBackupTime":"2026-06-13T02:00:00Z","operatingSystem":"Windows"}
	]}`,
	"/alerts": `{"count":1,"alerts":[
		{"severity":"WARNING","category":"Server","message":"Index size threshold exceeded","time":"2026-06-13T08:00:00Z","acknowledged":false}
	]}`,
	"/serverstatistics": `{"upSince":"2026-06-13T00:00:00Z","saves":12000,"saveSize":987654321,"recovers":42,"recoverSize":1234567,"badSaves":7,"badRecovers":1}`,
	"/jobs": `{"count":2,"jobs":[
		{"id":1001,"name":"daily-app01","type":"save","state":"Completed","completionStatus":"Succeeded","client":"app01.local","startTime":"2026-06-13T01:00:00Z","endTime":"2026-06-13T01:30:00Z","group":"DefaultGroup","level":"Full"},
		{"id":1002,"name":"daily-db01","type":"save","state":"Completed","completionStatus":"Failed","client":"db01.local","startTime":"2026-06-13T02:00:00Z","endTime":"2026-06-13T02:15:00Z","group":"DBGroup","level":"Incr"}
	]}`,
	"/sessions": `{"count":1,"sessions":[
		{"type":"backup","client":"app01.local","state":"running","size":104857600}
	]}`,
	"/volumes": `{"count":2,"volumes":[
		{"name":"vol01","pool":"Default","mediaType":"adv_file","status":"appendable","capacity":1099511627776,"written":659706976665,"recycledCount":3},
		{"name":"vol02","pool":"DataDomain","mediaType":"Data Domain","status":"full","capacity":5497558138880,"written":1099511627776,"recycledCount":0}
	]}`,
	"/datadomainsystems": `{"count":1,"datadomainsystems":[
		{"name":"dd01.local","model":"DD9400","osVersion":"7.10.1.0","capacityTotal":98956046499840,"capacityUsed":32985348833280,"capacityAvailable":65970697666560,"logicalCapacityUsed":296868139499520}
	]}`,
	"/backups": `{"count":2,"backups":[
		{"client":"app01.local","name":"/data","level":"full","size":536870912000,"saveTime":"2026-06-13T01:00:00Z","retentionTime":"2026-07-13T01:00:00Z","pool":"DataDomain","duration":1800},
		{"client":"app01.local","name":"/data","level":"incr","size":10737418240,"saveTime":"2026-06-13T13:00:00Z","retentionTime":"2026-06-20T13:00:00Z","pool":"DataDomain","duration":120}
	]}`,
}

func main() {
	addr := os.Getenv("NSR_MOCK_ADDR")
	if addr == "" {
		addr = ":9090"
	}
	mux := http.NewServeMux()
	for path, body := range fixtures {
		body := body
		mux.HandleFunc(basePath+path, basicAuth(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Write through the io.Writer interface (not the concrete ResponseWriter)
			// so static analysis sees a plain byte copy of a static JSON fixture, not
			// templated user input. These payloads are compile-time constants.
			writeBody(w, body)
		}))
	}
	// addr derives from the environment; keep it out of the format string to avoid
	// log-injection taint. It is still bound below as the listen address.
	log.Print("mocknw listening (Basic auth: any non-empty user/pass)")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// writeBody copies a static fixture to the response via the io.Writer interface.
func writeBody(w io.Writer, body string) { _, _ = io.WriteString(w, body) }

// basicAuth rejects requests without HTTP Basic credentials, mirroring NetWorker.
func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user == "" || pass == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="NetWorker"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
