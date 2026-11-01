package executor

import (
	"context"
	"strconv"
)

// Local edition: before a friend relay also forwards UDP, the relay server
// checks the friend's UDP port. A UDP relay passes on datagrams from any
// source address, so to a target that answers with more data than it gets it
// would be a reflector. The website opens UDP only after a check with the
// node's configuration has shown an MSBOOST node; a friend sends only the
// address. An MSBOOST node's UDP port speaks only Mieru and stays silent to
// anything else, so the relay server sends random bytes and the queries of
// common UDP services, and UDP stays off when anything answers.

const udpTargetScript = `if ! command -v python3 >/dev/null; then
  export DEBIAN_FRONTEND=noninteractive
  { apt-get update && apt-get install -y --no-install-recommends python3; } >/dev/null 2>&1 || true
fi
if ! command -v python3 >/dev/null; then
  printf 'MSBOOST_UDP_TARGET=unavailable\n'
  exit 0
fi
python3 - "$target_host" "$target_port" <<'PY'
import os, socket, sys, time
host, port = sys.argv[1], int(sys.argv[2])
probes = [
    os.urandom(64),
    bytes.fromhex('1234010000010000000000000000020001'),  # DNS: NS of the root
    b'\x00\x01\x00\x00\x00\x01\x00\x00stats\r\n',  # memcached
    b'M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: "ssdp:discover"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n',  # SSDP, Plex
    b'<?xml version="1.0" encoding="utf-8"?><soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing"><soap:Header><wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action><wsa:MessageID>urn:uuid:0</wsa:MessageID></soap:Header><soap:Body><Probe xmlns="http://schemas.xmlsoap.org/ws/2005/04/discovery"/></soap:Body></soap:Envelope>',  # WS-Discovery
    b'\x00\x01\x00\x00\x21\x12\xa4\x42' + os.urandom(12),  # STUN
    b'\x40\x01\x12\x34\xbb.well-known\x04core',  # CoAP
    b'\x17\x00\x03\x2a\x00\x00\x00\x00',  # NTP monlist
    b'\x1b' + bytes(47),  # NTP
    bytes.fromhex('302902010104067075626c6963a01c020412345678020100020100300e300c06082b060102010101000500'),  # SNMP
    b'\x02',  # SQL Server browser
    b'\x01\x00\x00\x00',  # Ubiquiti discovery
    b'\x00\x14\x00\x01\x03',  # Apple Remote Desktop
    b'\xff\xff\xff\xffTSource Engine Query\x00',  # game server queries
    b'\xff\xff\xff\xffgetstatus\n',
    b'd1:ad2:id20:' + os.urandom(20) + b'e1:q4:ping1:t2:aa1:y1:qe',  # BitTorrent DHT
    b'OPTIONS sip:a@a SIP/2.0\r\nVia: SIP/2.0/UDP a;branch=z9hG4bK1\r\nFrom: <sip:a@a>;tag=1\r\nTo: <sip:a@a>\r\nCall-ID: 1@a\r\nCSeq: 1 OPTIONS\r\nMax-Forwards: 70\r\nContent-Length: 0\r\n\r\n',  # SIP
    b'\x38' + os.urandom(8) + b'\x00\x00\x00\x00\x00',  # OpenVPN
]
s = socket.socket(socket.AF_INET6 if ':' in host else socket.AF_INET, socket.SOCK_DGRAM)
s.connect((host, port))
answered = refused = False
for _ in range(2):
    for probe in probes:
        try:
            s.send(probe)
        except ConnectionRefusedError:
            refused = True
        except OSError:
            pass
    end = time.monotonic() + 2
    while not answered and not refused and time.monotonic() < end:
        s.settimeout(max(0.05, end - time.monotonic()))
        try:
            s.recv(65535)
            answered = True
        except socket.timeout:
            break
        except ConnectionRefusedError:
            refused = True
        except OSError:
            break
    if answered or refused:
        break
print('MSBOOST_UDP_TARGET=' + ('answered' if answered else 'refused' if refused else 'silent'))
PY
`

// udpTargetReasons say why a friend relay stays TCP only.
var udpTargetReasons = map[string]string{
	"answered":    "朋友节点的 UDP 端口回应了探测，不像 MSBOOST 节点",
	"refused":     "朋友节点的 UDP 端口没开",
	"unavailable": "没能检查朋友节点的 UDP 端口",
}

// checkUDPTarget runs udpTargetScript on the relay server. It returns
// "silent" when UDP may be forwarded, "answered", "refused" when nothing
// listens there, or "unavailable" when the check could not run.
func (e *Engine) checkUDPTarget(ctx context.Context, s SSH, host string, port int) string {
	script := "set -Eeuo pipefail\nexport LC_ALL=C\n" + encodedAssignment("target_host", host) + encodedAssignment("target_port", strconv.Itoa(port)) + udpTargetScript
	out, err := e.Remote.Run(ctx, s, script)
	if v := marker(out, "MSBOOST_UDP_TARGET"); err == nil && (v == "silent" || v == "answered" || v == "refused") {
		return v
	}
	return "unavailable"
}
