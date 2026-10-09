import os,pty,sys,time,select,struct,fcntl,termios,signal,json
import pyte
cmd=json.loads(sys.argv[1]); steps=json.loads(sys.argv[2])
R,C=30,110
scr=pyte.Screen(C,R); st=pyte.ByteStream(scr)
pid,fd=pty.fork()
if pid==0:
    os.chdir('/tmp/probe-cwd'); os.environ['TERM']='xterm-256color'
    os.execvp(cmd[0],cmd)
fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',R,C,0,0))
def pump(t):
    end=time.time()+t
    while time.time()<end:
        r,_,_=select.select([fd],[],[],0.1)
        if r:
            try: d=os.read(fd,65536)
            except OSError: return
            if not d: return
            st.feed(d)
def show(tag):
    print(f"=== {tag}")
    lines=[l.rstrip() for l in scr.display]
    while lines and not lines[-1]: lines.pop()
    print("\n".join(lines[-14:]))
pump(float(os.environ.get('BOOT','6'))); show("boot")
for s in steps:
    if s[0]=='send': os.write(fd,s[1].encode().decode('unicode_escape').encode('latin1')); pump(s[2] if len(s)>2 else 1.0)
    elif s[0]=='show': show(s[1])
try: os.kill(pid,signal.SIGKILL)
except: pass
if os.environ.get('COLORS'):
    from collections import Counter
    c=Counter()
    for y in range(R):
        for x in range(C):
            ch=scr.buffer[y][x]
            if ch.data.strip(): c[ch.fg]+=1
    print("FG",c.most_common(8))
