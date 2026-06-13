 #!/usr/bin/env bash
set -euo pipefail

# NetWorker CSV exports: Client Summary, Group Summary, Capacity
# Prérequis:
# - Exécuter depuis le serveur NMC ou une machine où gstclreport est installé
# - JAVA_HOME défini si nécessaire
# - nsrcapinfo disponible pour l’export capacity (NetWorker 9.2+)
# - Adapter USERNAME/PASSWORD/SERVER_NAME et le chemin GSTCLREPORT si besoin

USERNAME="${NW_USER:-administrator}"
PASSWORD="${NW_PASS:-changeme}"
SERVER_NAME="${NW_SERVER:-localhost}"
DAYS="${NW_DAYS:-60}"
OUTDIR="${1:-./nw_exports_$(date +%F_%H%M%S)}"
GSTCLREPORT="${GSTCLREPORT:-/opt/lgotnmc/bin/gstclreport}"
NSRCAPINFO="${NSRCAPINFO:-nsrcapinfo}"

mkdir -p "$OUTDIR"

if [[ -n "${JAVA_HOME:-}" ]]; then
  export JAVA_HOME
fi

log() { echo "[$(date '+%F %T')] $*"; }
warn() { echo "[$(date '+%F %T')] WARNING: $*" >&2; }

run_gst_report() {
  local report_path="$1"
  local outfile="$2"
  shift 2

  log "Export du rapport: $report_path -> $outfile"
  "$GSTCLREPORT" \
    -u "$USERNAME" \
    -P "$PASSWORD" \
    -r "$report_path" \
    -v table \
    -x csv \
    -f "$outfile" \
    "$@"
}

# 1) Client Summary CSV
# Exemple Dell/community: /Reports/Policy Statistics/Client Summary avec filtre Workflow Start Time
run_gst_report \
  "/Reports/Policy Statistics/Client Summary" \
  "$OUTDIR/client_summary.csv" \
  -C "Workflow Start Time" "${DAYS} days ago"

# 2) Group Summary CSV
# Selon versions/rapports, le chemin peut varier. On tente d’abord Policy Statistics puis fallback historique NetWorker Backup Statistics.
if ! run_gst_report \
  "/Reports/Policy Statistics/Group Summary/Daily Group Report" \
  "$OUTDIR/group_summary.csv" \
  -C "Workflow Start Time" "${DAYS} days ago"
then
  warn "Échec sur /Reports/Policy Statistics/Group Summary/Daily Group Report, tentative fallback..."
  run_gst_report \
    "/Reports/NetWorker Backup Statistics/Group Summary/Daily Group Report" \
    "$OUTDIR/group_summary.csv" \
    -C "Save Time" "${DAYS} days ago"
fi

# 3) Capacity CSV
# nsrcapinfo fournit une mesure de capacité sur 60 jours par défaut; -d permet d’étendre la fenêtre.
log "Export capacity avec nsrcapinfo -> $OUTDIR/capacity.csv"
if command -v "$NSRCAPINFO" >/dev/null 2>&1; then
  "$NSRCAPINFO" -d "$DAYS" > "$OUTDIR/capacity.txt"
else
  "$NSRCAPINFO" -d "$DAYS" > "$OUTDIR/capacity.txt"
fi

# Conversion texte -> pseudo CSV simple pour exploitation rapide
awk 'BEGIN{FS=":"; OFS=","}
     /^[A-Za-z]/ {
       key=$1; sub(/^[ \t]+/, "", key); sub(/[ \t]+$/, "", key);
       val=$2; sub(/^[ \t]+/, "", val); sub(/[ \t]+$/, "", val);
       gsub(/,/, " ", key); gsub(/,/, " ", val);
       if (length(key) > 0 && length(val) > 0) print key,val
     }' "$OUTDIR/capacity.txt" > "$OUTDIR/capacity.csv" || true

cat > "$OUTDIR/README.txt" <<EOT
Exports générés:
- client_summary.csv
- group_summary.csv
- capacity.txt
- capacity.csv

Variables utiles:
- NW_USER / NW_PASS / NW_SERVER
- NW_DAYS=60
- GSTCLREPORT=/opt/lgotnmc/bin/gstclreport
- NSRCAPINFO=nsrcapinfo

Exemple:
NW_USER=administrator NW_PASS='Secret123' NW_DAYS=90 bash networker_exports.sh /tmp/nw_reporting
EOT

log "Terminé. Fichiers disponibles dans: $OUTDIR"