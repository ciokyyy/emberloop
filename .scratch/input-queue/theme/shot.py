import os,pty,sys,time,select,struct,fcntl,termios,signal,json
import pyte
from PIL import Image,ImageDraw,ImageFont
cmd=json.loads(sys.argv[1]); out=sys.argv[2]; steps=json.loads(sys.argv[3]) if len(sys.argv)>3 else []
R,C=int(os.environ.get('R','34')),int(os.environ.get('C','110'))
BG='1e1e2e';FG='cdd6f4'
pal=dict(black='45475a',red='f38ba8',green='a6e3a1',brown='f9e2af',blue='89b4fa',magenta='f5c2e7',cyan='94e2d5',white='bac2de',
 brightblack='585b70',brightred='f38ba8',brightgreen='a6e3a1',brightbrown='f9e2af',brightblue='89b4fa',brightmagenta='f5c2e7',brightcyan='94e2d5',brightwhite='a6adc8')
scr=pyte.Screen(C,R); st=pyte.ByteStream(scr)
pid,fd=pty.fork()
if pid==0:
    os.chdir(os.environ.get('CWD','/tmp/probe-cwd')); os.environ['TERM']='xterm-256color'; os.execvp(cmd[0],cmd)
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
pump(float(os.environ.get('BOOT','9')))
for s in steps:
    os.write(fd,s[0].encode().decode('unicode_escape').encode('latin1')); pump(s[1])
fnt=ImageFont.truetype('/usr/share/fonts/TTF/MesloLGMNerdFontMono-Regular.ttf',16) if os.path.exists('/usr/share/fonts/TTF/MesloLGMNerdFontMono-Regular.ttf') else ImageFont.load_default()
bold=ImageFont.truetype('/usr/share/fonts/TTF/MesloLGMNerdFontMono-Bold.ttf',16)
cw=int(fnt.getlength('M')); ch=22
def col(v,d):
    if v=='default': return '#'+d
    if v in pal: return '#'+pal[v]
    return '#'+v
img=Image.new('RGB',(C*cw,R*ch),'#'+BG); dr=ImageDraw.Draw(img)
for y in range(R):
    for x in range(C):
        c=scr.buffer[y][x]
        f,b=c.fg,c.bg
        if c.reverse: f,b=b,f
        bgc=col(b,BG)
        if bgc!='#'+BG: dr.rectangle([x*cw,y*ch,(x+1)*cw,(y+1)*ch],fill=bgc)
        if c.data.strip(): dr.text((x*cw,y*ch+2),c.data,font=bold if c.bold else fnt,fill=col(f,FG))
img.save(out)
try: os.kill(pid,signal.SIGKILL)
except: pass
if os.environ.get('DUMP'):
    n=int(os.environ['DUMP'])
    for y in range(R-n,R):
        runs=[];cur=None
        for x in range(C):
            c=scr.buffer[y][x]; k=(c.fg,c.bg,c.bold)
            if cur and cur[0]==k: cur[1]+=c.data
            else:
                cur=[k,c.data]; runs.append(cur)
        print(y, ' | '.join(f"{t!r}{'' if k[0]=='default' else ' fg='+k[0]}{'' if k[1]=='default' else ' bg='+k[1]}{' B' if k[2] else ''}" for k,t in [(r[0],r[1]) for r in runs] if t.strip() or k[1]!='default'))
