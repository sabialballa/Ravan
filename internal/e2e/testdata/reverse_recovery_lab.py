#!/usr/bin/env python3
"""Owned, isolated reverse transport comparison. No host routing/firewall edits."""
import argparse,base64,json,os,pathlib,re,signal,socket,subprocess,sys,threading,time,statistics

def run(*a,**kw):return subprocess.run(a,check=True,text=True,capture_output=True,**kw).stdout
def ns(n,*a):return ['ip','netns','exec',n,*a]
def emit(path,obj):
 with open(path,'a') as f:f.write(json.dumps(obj)+'\n');f.flush()

def echo(port=10001):
 s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('127.0.0.1',port));s.listen()
 def one(c):
  try:
   with c:
    while True:
     b=c.recv(65536)
     if not b:return
     c.sendall(b)
  except OSError:pass
 while True:
  c,_=s.accept();threading.Thread(target=one,args=(c,),daemon=True).start()

def driver(host,path,port=11001):
 lock=threading.Lock();stats=dict(bytes=0,errors=0,corrupt=0,probe_ok=0,probe_fail=0,last_ok=0,latencies=[],scheduler_lag_ms=0)
 events=pathlib.Path(path).with_name(pathlib.Path(path).name.replace('-samples','-probes'))
 def schedule_watch():
  while True:
   began=time.monotonic();time.sleep(.1);lag=max(0,time.monotonic()-began-.1)*1000
   with lock:stats['scheduler_lag_ms']=max(stats['scheduler_lag_ms'],lag)
 def receive(c,n):
  out=bytearray()
  while len(out)<n:
   b=c.recv(n-len(out))
   if not b:raise OSError('EOF')
   out.extend(b)
  return out
 def worker(i):
  size=int(os.environ.get('RAVAN_LAB_PAYLOAD_BYTES',65536))
  payload=(bytes([(v+i)%256 for v in range(256)])*((size+255)//256))[:size]
  while True:
   try:
    with socket.create_connection((host,port),timeout=2) as c:
     c.settimeout(2)
     while True:
      began=time.monotonic();c.sendall(payload);got=receive(c,len(payload))
      with lock:
       if got!=payload:stats['corrupt']+=1;raise OSError('corrupt data')
       stats['bytes']+=2*len(payload)
      # 512 KiB/s of payload per worker, identical offered load per mode.
      time.sleep(max(0,len(payload)/int(os.environ.get('RAVAN_LAB_WORKER_BPS',512*1024))-(time.monotonic()-began)))
   except OSError:
    with lock:stats['errors']+=1
    time.sleep(.1)
 def probe():
  while True:
   start=time.monotonic();wall_start=time.time();ok=False;phase='connect';failure=None
   try:
    with socket.create_connection((host,port),timeout=2) as c:
     c.settimeout(2);payload=os.urandom(32);phase='send';c.sendall(payload);phase='receive';ok=receive(c,32)==payload
     if not ok:failure={'type':'payload_mismatch'}
   except OSError as exc:failure={'type':type(exc).__name__,'errno':exc.errno,'message':str(exc)}
   with lock:
    stats['probe_ok' if ok else 'probe_fail']+=1
    if ok:stats['last_ok']=time.time();stats['latencies'].append((time.monotonic()-start)*1000)
   if not ok:
    with lock:lag=stats['scheduler_lag_ms']
    emit(events,{'time':time.time(),'monotonic':time.monotonic(),'phase':phase,'elapsed_ms':(time.monotonic()-start)*1000,'wall_elapsed_ms':(time.time()-wall_start)*1000,'scheduler_lag_ms':lag,'failure':failure})
   time.sleep(max(0,.5-(time.monotonic()-start)))
 for i in range(2):threading.Thread(target=worker,args=(i,),daemon=True).start()
 threading.Thread(target=probe,daemon=True).start()
 threading.Thread(target=schedule_watch,daemon=True).start()
 while True:
  with lock:
   obj={k:v for k,v in stats.items() if k!='latencies'};lat=stats['latencies'];stats['latencies']=[]
   obj.update(time=time.time(),monotonic=time.monotonic(),latency_median_ms=statistics.median(lat) if lat else None)
   stats['scheduler_lag_ms']=0
  emit(path,obj);time.sleep(1)

class Lab:
 def __init__(self,args):self.a=args;self.w=pathlib.Path(args.work);self.w.mkdir(mode=0o700,parents=True);self.procs=[];self.engines={};self.modes=['pck','wssmux'];self.stop=False
 def spawn(self,cmd,log):
  f=open(log,'ab');p=subprocess.Popen(cmd,stdout=f,stderr=f,env=dict(os.environ,GOMAXPROCS='2'));f.close();self.procs.append(p);return p
 def config(self,tr,role):
  i=self.modes.index(tr)+1;addr=f'10.99.{i}.1';section='server' if role=='server' else 'client'
  common=f'''transport = "{tr}"
token = "owned-reverse-recovery-test-0123456789abcdef"
log_level = "info"
skip_optz = true
keepalive_period = 75
mux_version = 2
mux_session = 8
mux_framesize = 32768
mux_recievebuffer = 4194304
mux_streambuffer = 2097152
kcp_mtu = 1350
kcp_interval = 20
kcp_resend = 2
kcp_nodelay = 1
kcp_nocongestion = 1
kcp_sndwnd = 1024
kcp_rcvwnd = 1024
'''
  if role=='server':extra=f'bind_addr = "{addr}:9000"\nports = ["11001=127.0.0.1:10001"]\nheartbeat = 10\nchannel_size = 2048\n'
  else:extra=f'remote_addr = "{addr}:9000"\nconnection_pool = 4\nretry_interval = 1\ndial_timeout = 5\n'
  if tr=='pck':extra+='pck_interface = "vi"\n'
  elif role=='server':extra+=f'tls_cert = "{self.w}/cert.pem"\ntls_key = "{self.w}/key.pem"\n'
  p=self.w/f'{tr}-{role}.toml';p.write_text(f'[{section}]\n'+common+extra);p.chmod(0o600);return p
 def start_engine(self,tr,role,binary=None):
  if role=='client' and not binary:binary=getattr(self.a,'client_binary',None)
  n=tr+('-i' if role=='server' else '-o');p=self.spawn(ns(n,binary or self.a.binary,'-c',str(self.config(tr,role))),self.w/f'{tr}-{role}.log');self.engines[(tr,role)]=p
 def setup(self):
  # Refuse accidental host-network execution, including a mount-only unshare.
  if os.readlink('/proc/self/ns/net')==self.a.parent_net or os.readlink('/proc/self/ns/mnt')==self.a.parent_mount:raise RuntimeError('separate network AND mount namespaces required')
  run('mount','-t','tmpfs','none','/run');pathlib.Path('/run/netns').mkdir(exist_ok=True)
  run('openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-keyout',str(self.w/'key.pem'),'-out',str(self.w/'cert.pem'),'-days','2','-subj','/CN=reverse-lab.invalid','-addext','subjectAltName=DNS:reverse-lab.invalid')
  for i,tr in enumerate(self.modes,1):
   for suffix in ['i','o']:run('ip','netns','add',tr+'-'+suffix)
   run('ip','link','add','v'+str(i)+'a','type','veth','peer','name','v'+str(i)+'b')
   for suffix,dev,last in [('i','v'+str(i)+'a',1),('o','v'+str(i)+'b',2)]:
    n=tr+'-'+suffix;run('ip','link','set',dev,'netns',n);run('ip','-n',n,'link','set',dev,'name','vi');run('ip','-n',n,'addr','add',f'10.99.{i}.{last}/24','dev','vi');run('ip','-n',n,'link','set','vi','up');run('ip','-n',n,'link','set','lo','up')
    run(*ns(n,'tc','qdisc','add','dev','vi','root','netem','delay','20ms'));run(*ns(n,'tc','qdisc','add','dev','vi','clsact'))
   self.spawn(ns(tr+'-o',sys.executable,__file__,'echo'),self.w/f'{tr}-echo.log')
   self.start_engine(tr,'server');time.sleep(.3);self.start_engine(tr,'client')
   self.spawn(ns(tr+'-i',sys.executable,__file__,'driver',f'10.99.{i}.1',str(self.w/f'{tr}-samples.jsonl')),self.w/f'{tr}-driver.log')
 def latest(self,tr):
  p=self.w/f'{tr}-samples.jsonl'
  try:return json.loads(p.read_text().splitlines()[-1])
  except (OSError,IndexError,json.JSONDecodeError):return {}
 def ports(self,tr):
  i=self.modes.index(tr)+1
  if tr=='pck':return sorted(set(map(int,re.findall(rf'src=10\.99\.{i}\.2:(\d+)',(self.w/f'{tr}-client.log').read_text()))))
  text=run(*ns(tr+'-o','ss','-Hnt','state','established'));return sorted(set(map(int,re.findall(rf'10\.99\.{i}\.2:(\d+)',text))))
 def filter(self,tr,kind):
  direction='i' if kind=='return' else 'o';n=tr+'-'+direction
  if kind=='both':
   for d in ['i','o']:run(*ns(tr+'-'+d,'tc','filter','add','dev','vi','egress','protocol','ip','pref','10','u32','match','ip','protocol','6','0xff','action','drop'))
   return []
  ports=self.ports(tr)
  if not ports:raise RuntimeError('could not identify active flow ports for '+tr)
  for port in ports:run(*ns(n,'tc','filter','add','dev','vi','egress','protocol','ip','pref','10','u32','match','ip','protocol','6','0xff','match','ip','dport' if kind=='return' else 'sport',str(port),'0xffff','action','drop'))
  return ports
 def clear(self,tr):
  for d in ['i','o']:subprocess.run(ns(tr+'-'+d,'tc','filter','del','dev','vi','egress','pref','10'),capture_output=True)
 def perform(self,kind):
  start=time.time();record={'kind':kind,'started':start,'modes':{}}
  for tr in self.modes:
   before=self.latest(tr);item={'before':before,'lost_seen':False,'application_recovery_seconds':None,'control_claims_before':(self.w/f'{tr}-server.log').read_text().count('control channel successfully established')}
   if kind in ['outgoing','return','both']:item['blocked_ports']=self.filter(tr,kind)
   else:
    role='server' if kind=='server_crash' else 'client';p=self.engines[(tr,role)];p.kill();p.wait();item['crashed_role']=role
   item['applied_at']=time.time();record['modes'][tr]=item
  end_fault=15 if kind=='both' else 5 if kind.endswith('crash') else 120
  cleared=False
  for _ in range(150):
   age=time.time()-start
   if self.stop:break
   if age>=end_fault and not cleared:
    for tr in self.modes:
     self.clear(tr)
     if kind.endswith('crash'):self.start_engine(tr,record['modes'][tr]['crashed_role'])
    cleared=True;record['fault_removed_after_seconds']=age
   for tr,item in record['modes'].items():
    now=self.latest(tr)
    if not item['lost_seen'] and now.get('probe_fail',0)>item['before'].get('probe_fail',0):item['lost_seen']=True;item['loss_observed_at']=now['time']
    if item['lost_seen'] and now.get('last_ok',0)>max(item['loss_observed_at'],item['applied_at']+1) and item['application_recovery_seconds'] is None:item['application_recovery_seconds']=now['last_ok']-item['applied_at']
   if cleared and all(v['application_recovery_seconds'] is not None or not v['lost_seen'] for v in record['modes'].values()):break
   time.sleep(1)
  for tr,item in record['modes'].items():
   item['after']=self.latest(tr);item['ports_after']=self.ports(tr);item['control_claims_after']=(self.w/f'{tr}-server.log').read_text().count('control channel successfully established')
   self.clear(tr)
  record['ended']=time.time();emit(self.w/'faults.jsonl',record);return record
 def main(self):
  self.setup();deadline=time.monotonic()+60
  while time.monotonic()<deadline and not all(self.latest(t).get('probe_ok',0)>5 for t in self.modes):time.sleep(1)
  if not all(self.latest(t).get('probe_ok',0)>5 for t in self.modes):raise RuntimeError('warmup failed; inspect engine logs')
  began=time.time();began_mono=time.monotonic();emit(self.w/'progress.jsonl',{'event':'begin','time':began,'seconds':self.a.seconds})
  schedule=[(300*(i+1),k) for i,k in enumerate(['outgoing','return','both','server_crash','client_crash']*2)] if self.a.seconds>=3600 else [(self.a.fault_offset,self.a.fault_kind)]
  next_event=0;next_report=0
  while not self.stop and time.monotonic()-began_mono<self.a.seconds:
   age=time.monotonic()-began_mono
   if next_event<len(schedule) and age>=schedule[next_event][0]:
    record=self.perform(schedule[next_event][1]);next_event+=1;emit(self.w/'progress.jsonl',{'event':'fault_finished','time':time.time(),'kind':record['kind']})
   if age>=next_report:
    emit(self.w/'progress.jsonl',{'event':'sample','elapsed':age,'modes':{t:self.latest(t) for t in self.modes}});next_report+=60
   time.sleep(1)
  final={'elapsed_seconds':time.monotonic()-began_mono,'complete':not self.stop,'modes':{t:self.latest(t) for t in self.modes},'fault_count':next_event};(self.w/'result.json').write_text(json.dumps(final,indent=2));print(json.dumps(final),flush=True)
 def close(self):
  for p in reversed(self.procs):
   if p.poll() is None:p.terminate()
  for p in self.procs:
   try:p.wait(timeout=3)
   except subprocess.TimeoutExpired:p.kill();p.wait()

if __name__=='__main__':
 if len(sys.argv)>1 and sys.argv[1]=='echo':echo(int(sys.argv[2]) if len(sys.argv)>2 else 10001);sys.exit()
 if len(sys.argv)>1 and sys.argv[1]=='driver':driver(sys.argv[2],sys.argv[3],int(sys.argv[4]) if len(sys.argv)>4 else 11001);sys.exit()
 p=argparse.ArgumentParser();p.add_argument('--binary',required=True);p.add_argument('--client-binary');p.add_argument('--work',required=True);p.add_argument('--seconds',type=int,default=3600);p.add_argument('--fault-offset',type=int,default=5);p.add_argument('--fault-kind',choices=['outgoing','return','both','server_crash','client_crash'],default='outgoing');p.add_argument('--parent-net',required=True);p.add_argument('--parent-mount',required=True);a=p.parse_args();lab=Lab(a)
 signal.signal(signal.SIGTERM,lambda *_:setattr(lab,'stop',True))
 try:lab.main()
 finally:lab.close()
