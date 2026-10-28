"""Bounded persistent offline decoder; never owns a game connection or world cache."""
import atexit
import json
import queue
import subprocess
import threading

MAX_FRAME=32<<20


class CodecWorker:
    def __init__(self,binary):
        self.binary=str(binary)
        self.lock=threading.Lock()
        self.process=None

    def _start(self):
        self.process=subprocess.Popen([self.binary,'--stream'],stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,bufsize=65536)
        process=self.process
        self.replies=queue.Queue(maxsize=1)
        replies=self.replies
        def read():
            try:
                while True:
                    line=process.stdout.readline(MAX_FRAME+1)
                    if not line or len(line)>MAX_FRAME:
                        replies.put(None,timeout=.1);return
                    replies.put(line,timeout=.1)
            except (OSError,ValueError):
                try:replies.put(None,timeout=.1)
                except queue.Full:pass
            except queue.Full:return
        threading.Thread(target=read,daemon=True,name='world-codec-reader').start()

    def _close(self):
        process=self.process
        self.process=None
        if process is None:return
        if process.poll() is None:
            process.kill()
        process.wait(timeout=2)
        for stream in (process.stdin,process.stdout):
            try:stream.close()
            except (OSError,ValueError):pass

    def close(self):
        with self.lock:self._close()

    def call(self,payload,timeout=15):
        frame=json.dumps(payload,separators=(',',':')).encode()+b'\n'
        if len(frame)>MAX_FRAME:raise ValueError('原生解码请求超出上限。')
        with self.lock:
            try:
                if self.process is None or self.process.poll() is not None:
                    self._close();self._start()
                process,replies=self.process,self.replies
                def write():
                    try:
                        # FileIO.write may only consume part of a large frame.
                        remaining=memoryview(frame)
                        while remaining:
                            written=process.stdin.write(remaining)
                            if not written:raise OSError('closed codec input')
                            remaining=remaining[written:]
                        process.stdin.flush()
                    except (OSError,ValueError):
                        try:replies.put(None,timeout=.1)
                        except queue.Full:pass
                threading.Thread(target=write,daemon=True,name='world-codec-input').start()
                line=self.replies.get(timeout=timeout)
                if line is None:raise ValueError('原生解码进程未返回结果。')
                response=json.loads(line)
                if not isinstance(response,dict) or not isinstance(response.get('result'),dict) or 'error' in response:
                    raise ValueError('原生解码失败，未确认内部数据。')
                return response['result']
            except (OSError,ValueError,queue.Empty):
                self._close()
                raise ValueError('原生解码失败或超时，未确认内部数据。') from None


_workers={}
_lock=threading.Lock()


def request(binary,payload):
    key=str(binary)
    with _lock:
        if key not in _workers:_workers[key]=CodecWorker(binary)
        worker=_workers[key]
    return worker.call(payload)


def close_workers():
    with _lock:
        for worker in _workers.values():worker.close()
        _workers.clear()


atexit.register(close_workers)
