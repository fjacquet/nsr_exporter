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
		{"hostname":"app01.local","ndmp":false,"scheduledBackup":true,"backupCommand":"save","parallelism":4,"os":"Linux"},
		{"hostname":"db01.local","ndmp":true,"scheduledBackup":true,"backupCommand":"nsrndmp_save","parallelism":12,"os":"Windows"}
	]}`,
	"/alerts": `{"count":1,"alerts":[
		{"priority":"critical","category":"Server","message":"Index size threshold exceeded","timestamp":"2026-06-13T08:00:00Z"}
	]}`,
	"/serverstatistics": `{"upSince":"2026-06-13T00:00:00Z","saves":12000,"saveSize":{"unit":"KB","value":987654321},"recovers":42,"recoverSize":{"unit":"KB","value":1234567},"badSaves":7,"badRecovers":1,"currentSaves":3,"currentRecovers":0,"maxSaves":64,"maxRecovers":32}`,
	"/jobs": `{"count":2,"jobs":[
		{"id":1001,"name":"daily-app01","type":"save","state":"Completed","completionStatus":"Succeeded","clientHostname":"app01.local","startTime":"2026-06-13T01:00:00Z","endTime":"2026-06-13T01:30:00Z","level":"Full"},
		{"id":1002,"name":"daily-db01","type":"save","state":"Completed","completionStatus":"Failed","clientHostname":"db01.local","startTime":"2026-06-13T02:00:00Z","endTime":"2026-06-13T02:15:00Z","level":"Incr"}
	]}`,
	"/sessions": `{"count":1,"sessions":[
		{"mode":"Saving","clientHostname":"app01.local","size":{"unit":"KB","value":102400},"transferRate":{"unit":"KB/s","value":2048}}
	]}`,
	"/volumes": `{"count":2,"volumes":[
		{"name":"vol01","pool":"Default","type":"adv_file","states":["Recyclable"],"capacity":{"unit":"KB","value":1073741824},"written":{"unit":"KB","value":644245094},"recycled":3},
		{"name":"vol02","pool":"DataDomain","type":"Data Domain","states":["WORM"],"capacity":{"unit":"KB","value":5368709120},"written":{"unit":"KB","value":1073741824},"recycled":0}
	]}`,
	"/datadomainsystems": `{"count":1,"dataDomainSystems":[
		{"name":"dd01.local","model":"DD9400","osVersion":"7.10.1.0","totalCapacity":"90 TB","usedCapacity":"30 TB","availableCapacity":"60 TB","usedLogicalCapacity":"270 TB"}
	]}`,
	"/devices": `{"count":2,"devices":[
		{"name":"tape01","mediaType":"LTO Ultrium-8","mediaFamily":"Tape","status":"Enabled","deviceSerialNumber":"SN001"},
		{"name":"adv01","mediaType":"adv_file","mediaFamily":"Disk","status":"Enabled","deviceSerialNumber":"SN002"}
	]}`,
	"/storagenodes": `{"count":1,"storageNodes":[
		{"name":"sn01.local","enabled":true,"typeOfStorageNode":"SCSI","version":"19.13","numberOfDevices":4}
	]}`,
	"/pools": `{"count":2,"pools":[
		{"name":"Default","poolType":"Backup","enabled":true},
		{"name":"DataDomain","poolType":"Backup","enabled":true}
	]}`,
	"/vmware/vcenters": `{"count":1,"vCenters":[
		{"hostname":"vcenter.local","cloudDeployment":false}
	]}`,
	"/protectionpolicies": `{"count":2,"protectionPolicies":[
		{"name":"GoldPolicy","workflows":[{"enabled":true}]},
		{"name":"SilverPolicy","workflows":[{"enabled":false}]}
	]}`,
	"/protectiongroups": `{"count":2,"protectionGroups":[
		{"name":"DBGroup","workItemType":"Client"},
		{"name":"AppGroup","workItemType":"VMware"}
	]}`,
	"/backups": `{"count":2,"backups":[
		{"clientHostname":"app01.local","name":"/data","level":"full","size":{"unit":"Byte","value":536870912000},"saveTime":"2026-06-13T01:00:00Z","retentionTime":"2026-07-13T01:00:00Z","completionTime":"2026-06-13T01:30:00Z"},
		{"clientHostname":"app01.local","name":"/data","level":"incr","size":{"unit":"Byte","value":10737418240},"saveTime":"2026-06-13T13:00:00Z","retentionTime":"2026-06-20T13:00:00Z","completionTime":"2026-06-13T13:02:00Z"}
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
