// Optional maintainer tool: npx tsx scripts/helper-fixtures.ts; no JS runtime needed by Go.
import {writeFileSync} from 'node:fs';
import {safeNumber,safeNumberOrNull,toNumber,toNumberSafe,toFiniteNumberOrNull,parseResponse,decodeGBK} from '../.upstream/src/core/parser';
import {parseMarketTime,buildTimeMeta,buildTimeMetaFromDateAndTime,formatInTz,MARKET_TZ} from '../.upstream/src/core/time';
import {parseJsVars} from '../.upstream/src/core/jsVars';
import {extractJsonFromJsonp} from '../.upstream/src/core/jsonp';
import {generateSearchExternalLinks} from '../.upstream/src/externalLinks';
import {chunkArray,asyncPool} from '../.upstream/src/core/utils';
import {MemoryCache,createCacheKey} from '../.upstream/src/core/cache';
const fixtures:any[]=[];
function add(kind:string,input:any,expected:any){fixtures.push({kind,input,expected})}
const clean=(v:any):any=>typeof v==='number'&&!Number.isFinite(v)?String(v):v;
for(const value of ['', ' ', '-', '--', '0','-0','12.3%','-4元',' .5rest','1e','1e+2tail','0x10','0b11','0o77','1_000','Infinity','+Infinity','-Infinity','Infinityx','1e309','-1e309','NaN','abc','\u200312','\ufeff12','0x10000000000000000','+0x10']){
 for(const [kind,fn]of Object.entries({safeNumber,safeNumberOrNull,toNumber,toNumberSafe,toFiniteNumberOrNull}))add(kind,value,clean(fn(value)))
}
for(const value of [null,true,false,0,1.5,-7,[],[1],[1,2],[null,2],[[1]],{}, {x:1}])add('toNumberSafe',value,clean(toNumberSafe(value)));
for(const value of [null,1.5,-7])add('toFiniteNumberOrNull',value,toFiniteNumberOrNull(value as any));
for(const value of ['', 'nothing', ' v_sh600519="1~茅台~"; v_hk00700="2~腾讯";','a=;empty="";one=";bad;raw = 12; = x;', 'var v_a="x~y";foo=bar=baz;'])add('parseResponse',value,parseResponse(value));
for(const bytes of [[],[0xc4,0xe3,0xba,0xc3],[0x81],[0xff],[0x80],[65,0,66],[0x81,0xff],[0xff,0xff],[0xa1,0x40],[0xa2,0xab],[0xfe,0xfe]])add('decodeGBK',bytes,decodeGBK(Uint8Array.from(bytes).buffer));
for(const tz of Object.values(MARKET_TZ))for(const local of ['2024-07-09 09:30','2024-01-09 16:05','2024-11-03 01:30','2024-03-10 02:30','20240229','2024/07/09 09:30:01','2024-07-09T09:30','bad','2024-02-30','']){
 add('buildTimeMeta',{local,tz},buildTimeMeta(local,tz));const epoch=parseMarketTime(local,tz);add('formatInTZ',{epoch:Number.isFinite(epoch)?epoch:null,tz},formatInTz(epoch,tz))
}
for(const tz of Object.values(MARKET_TZ))for(const [date,clock]of [['20240709','09:30'],['2024-07-09','16:00:01'],['bad','09:30'],['2024-02-30','00:00']])add('buildTimeMetaFromDateAndTime',{date,clock,tz},buildTimeMetaFromDateAndTime(date,clock,tz));
for(const text of [`var x={"a":[1,";"]};let y=false;const z=null;`, `var x='单引号';var y=[['a;\"b','it\\'s'],{"x":'value'}];`, `var x=[1].concat([2]);var y={bad:1};var z=undefined;`, `var x="first";var x="second";`, `var x="back\\\\slash";var y='double"quote';var z='line\\nfeed';`])add('parseJSVars',text,parseJsVars(text,['x','y','z']));
for(const text of ['cb({"x":1});','/*script*/\ncb([1,2])','foo.bar(null)','_cb("a(b)")','invalid','cb({invalid})']){try{add('extractJSONFromJSONP',text,{value:extractJsonFromJsonp(text)})}catch(e:any){add('extractJSONFromJSONP',text,{error:e.code})}}
for(const [code,market]of [['600519','sh'],['000001','0'],['sh000001','1'],['399001','sz'],['700','hk'],['116.700','116'],['usAAPL.OQ','us'],['USB',''],['105.msft','105'],['IXIC','100'],['100.NDX100','global'],['DJI','global'],['bj920001',''],['BK0001',''],['SHFE.rb2501',''],['weird name?&','other'],['sh600519','hk'],['shsh600519','sh'],['AAPL',''],[' sh600519 ',' SH '],['100.IXIC',' 100 '],['a+b /!~*\'()','?']]){const value={code,market,name:'x',type:'stock',category:''};add('externalLinks',value,generateSearchExternalLinks(value as any))}
for(const input of [{items:[],size:1},{items:[1,2,3,4,5],size:2},{items:[1,2],size:5}])add('chunkArray',input,chunkArray(input.items,input.size));
for(const parts of [[],['x',null,'',false,0,true],['a:b','c'],[1e-7,1e21,-0],['中文',' ','∅']])add('cacheKey',parts,createCacheKey(...parts));
async function main(){
 for(const ordered of [true,false])for(const concurrency of [1,2,5]){
  const tasks=[0,1,2,3].map(i=>async()=>i*2);
  add('asyncPool',{ordered,concurrency,values:[0,2,4,6]},await asyncPool(tasks,concurrency,ordered))
 }
 writeFileSync(new URL('../testdata/helpers-parity.json',import.meta.url),JSON.stringify(fixtures,null,2)+'\n');
 const realNow=Date.now;let now=100000;Date.now=()=>now;
 const actions:any[]=[{op:'set',key:'',value:'empty'},{op:'set',key:'a',value:1},{op:'get',key:''},{op:'set',key:'b',value:2},{op:'get',key:'a'},{op:'get',key:''},{op:'set',key:'',value:'changed'},{op:'size'},{op:'set',key:'nil',value:null,ttl:0},{op:'get',key:'nil'},{op:'get',key:'b'},{op:'advance',ms:10},{op:'get',key:''},{op:'advance',ms:1},{op:'get',key:''},{op:'size'},{op:'set',key:'c',value:3},{op:'set',key:'d',value:4,ttl:2},{op:'configure',maxSize:1},{op:'size'},{op:'advance',ms:3},{op:'cleanup'},{op:'size'},{op:'delete',key:'missing'},{op:'set',key:'z',value:9,ttl:0},{op:'delete',key:'z'},{op:'clear'},{op:'size'}];
 const cache=new MemoryCache({defaultTTL:10,maxSize:2});const trace:any[]=[];
 for(const a of actions){let expected:any=null;switch(a.op){case 'advance':now+=a.ms;break;case 'set':cache.set(a.key,a.value,a.ttl);break;case 'get':const value=cache.get(a.key);expected={found:value!==undefined,value:value??null};break;case 'size':expected=cache.size;break;case 'configure':cache.configure({maxSize:a.maxSize});break;case 'cleanup':expected=cache.cleanup();break;case 'delete':expected=cache.delete(a.key);break;case 'clear':cache.clear();break;}trace.push({...a,now,expected});now++}
 Date.now=realNow;writeFileSync(new URL('../testdata/cache-parity.json',import.meta.url),JSON.stringify(trace,null,2)+'\n');console.log(`Generated ${fixtures.length} public helper fixtures and ${trace.length} cache operations`)
}
main().catch(e=>{console.error(e);process.exitCode=1});
