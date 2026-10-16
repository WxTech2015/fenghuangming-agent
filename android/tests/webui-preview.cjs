const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../build/webroot');
const fixture = `
const state={endpoint:'https://music.example.com',hasToken:true,authorized:true,serverConnected:true,playerConnected:true,playerReady:true,emergencyStopped:false,cacheMaxMb:512,connections:4,current:{playbackId:'fixture_play',title:'测试歌曲 · 本机预览',artists:['界面测试']},player:{playbackId:'fixture_play',position:68,durationSeconds:180,paused:false,seekable:true,output:'模拟扬声器',systemVolume:9},overview:{waitingCount:2,queue:[{id:'a',title:'待播歌曲 A',artists:['测试歌手']},{id:'b',title:'待播歌曲 B',artists:['测试歌手']}]}};
window.fixtureRequests=[];
window.ksu={exec(command,options,callback){
 let result={ok:true};
 if(command.includes(' request ')){
  const payload=JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(command.split(' request ')[1]),c=>c.charCodeAt(0))));
  window.fixtureRequests.push(payload);
  if(payload.action==='state')result=structuredClone(state);
  if(payload.action==='logs')result={lines:['本机预览：未连接手机、未执行 Root 命令']};
  if(['seek','pause','resume','stop'].includes(payload.action)&&payload.playbackId!==state.player.playbackId)result={ok:false,error:'播放任务已改变'};
  else if(payload.action==='seek')state.player.position=payload.value;
  if(payload.action==='pause')state.player.paused=true;
  if(payload.action==='resume')state.player.paused=false;
  if(payload.action==='brake'){state.emergencyStopped=true;state.playerReady=false;state.player.paused=true;}
  if(payload.action==='release'){state.emergencyStopped=false;state.playerReady=true;}
 }
 const output=JSON.stringify(result);
 if(new URLSearchParams(location.search).has('sync'))return output;
 setTimeout(()=>window[callback](0,output,''),0);
}};
`;
const server = http.createServer((request, response) => {
  const url = new URL(request.url, 'http://localhost');
  if (url.pathname === '/fixture.js') { response.writeHead(200, {'Content-Type':'application/javascript'});response.end(fixture);return; }
  const name = url.pathname === '/' ? 'index.html' : decodeURIComponent(url.pathname.slice(1));
  const file = path.resolve(root, name);
  if (!file.startsWith(root + path.sep)) { response.writeHead(403);response.end();return; }
  try {
    let bytes = fs.readFileSync(file);
    if (name === 'index.html') bytes = Buffer.from(bytes.toString().replace('<head>', '<head><script src="./fixture.js"></script>'));
    const mime = name.endsWith('.html') ? 'text/html' : name.endsWith('.js') ? 'application/javascript' : name.endsWith('.css') ? 'text/css' : 'application/octet-stream';
    response.writeHead(200,{'Content-Type':mime});response.end(bytes);
  } catch { response.writeHead(404);response.end(); }
});
server.listen(5191,'127.0.0.1',()=>console.log('WebUI 测试预览 http://127.0.0.1:5191；模拟数据，不执行 Root 命令'));
