#!/bin/bash
set -euo pipefail
tr="$1"
: "${BIN:?test binary required}" "${W:?private working directory required}"
: "${RAVAN_PARENT_NETNS:?run through TestRawReverseTransports}" "${RAVAN_PARENT_MNTNS:?run through TestRawReverseTransports}"
if [[ "$(readlink /proc/self/ns/net)" == "$RAVAN_PARENT_NETNS" || "$(readlink /proc/self/ns/mnt)" == "$RAVAN_PARENT_MNTNS" ]]; then
 echo 'Refusing to run without separate network AND mount namespaces.' >&2
 exit 2
fi
mount -t tmpfs none /run
mkdir -p /run/netns
ip netns add iran
ip netns add outside
ip link add vi type veth peer name vk
ip link set vi netns iran
ip link set vk netns outside
ip -n iran addr add 10.99.0.1/24 dev vi
ip -n outside addr add 10.99.0.2/24 dev vk
for ns in iran outside; do ip -n "$ns" link set lo up; done
ip -n iran link set vi up
ip -n outside link set vk up
pids=()
cleanup(){ for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done; }
trap cleanup EXIT
ip netns exec outside python3 -u - <<'PY' > "$W/backend.log" 2>&1 &
import socket,threading
s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('127.0.0.1',10001));s.listen()
def echo(c):
 with c:
  while True:
   data=c.recv(65536)
   if not data:return
   c.sendall(data)
while True:
 c,_=s.accept();threading.Thread(target=echo,args=(c,),daemon=True).start()
PY
pids+=("$!")
ip netns exec iran env RAVAN_RAW_ROLE=server RAVAN_RAW_TRANSPORT="$tr" "$BIN" -test.run '^TestRawReverseNode$' -test.timeout=160s > "$W/server.log" 2>&1 &
pids+=("$!")
sleep .5
ip netns exec outside env RAVAN_RAW_ROLE=client RAVAN_RAW_TRANSPORT="$tr" "$BIN" -test.run '^TestRawReverseNode$' -test.timeout=160s > "$W/client.log" 2>&1 &
pids+=("$!")
if ! ip netns exec iran python3 - "$tr" <<'PY'
import socket,time,os,sys,threading
payload=os.urandom(256*1024);deadline=time.monotonic()+100;last=None
while time.monotonic()<deadline:
 try:
  with socket.create_connection(('10.99.0.1',11001),timeout=5) as c:
   c.settimeout(10)
   sender=threading.Thread(target=c.sendall,args=(payload,));sender.start()
   got=bytearray()
   while len(got)<len(payload):
    part=c.recv(len(payload)-len(got))
    if not part:raise RuntimeError('truncated payload')
    got.extend(part)
   sender.join(timeout=5)
   if sender.is_alive() or got!=payload:raise RuntimeError('corrupted data')
   print('RAW_REVERSE_PASS',sys.argv[1],len(payload),'bytes each way');sys.exit(0)
 except Exception as e:last=e;time.sleep(.25)
raise SystemExit(str(last))
PY
then
 tail -n 35 "$W/server.log" "$W/client.log"
 exit 1
fi
