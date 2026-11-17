// Run: npx tsx scripts/indicator-fixtures.ts. Go tests need only the generated JSON.
import {writeFileSync} from 'node:fs';
import * as api from '../.upstream/src/indicators';
const random=Array.from({length:320},(_,i)=>{const c=Math.round((90+i/17+Math.sin(i*0.37)*8+Math.cos(i*0.1)*4)*10000)/10000;return {open:c-0.5,high:c+1.3,low:c-1.6,close:c,volume:100+(i*173)%900}});
const datasets:any={random,flat:Array.from({length:60},()=>({open:10,high:10,low:10,close:10,volume:100})),down:Array.from({length:70},(_,i)=>({open:100-i,high:102-i,low:99-i,close:101-i,volume:100+i})),zero:Array.from({length:50},()=>({open:0,high:0,low:0,close:0,volume:0})),holes:random.slice(0,90).map((b,i)=>i%11===0?{...b,high:null,close:null,volume:null}:i%7===0?{...b,low:null}:b),leading:random.slice(0,90).map((b,i)=>i<9?{open:null,high:null,low:null,close:null,volume:null}:b),short:random.slice(0,1),empty:[]};
const custom:any={SMA:{period:3,decimals:0},EMA:{period:3,decimals:2},WMA:{period:4,decimals:6},MA:{periods:[2,5,9],type:'ema',decimals:2},MACD:{short:3,long:7,signal:4,decimals:2},BOLL:{period:5,stdDev:1.5,decimals:2},KDJ:{period:5,kPeriod:2,dPeriod:4,decimals:2},RSI:{periods:[2,5,9],decimals:2},WR:{periods:[2,5,9],decimals:2},BIAS:{periods:[2,5,9],decimals:2},CCI:{period:5,decimals:2},ATR:{period:5,decimals:2},OBV:{maPeriod:5},ROC:{period:5,signalPeriod:4},DMI:{period:5,adxPeriod:3},SAR:{afStart:0.03,afIncrement:0.01,afMax:0.15},KC:{emaPeriod:5,atrPeriod:3,multiplier:1.5}};
const closeFunctions=new Set(['SMA','EMA','WMA','MA','MACD','BOLL','RSI','BIAS']);
const fixtures:any[]=[];
for(const [dataset,data] of Object.entries(datasets) as any){for(const [name,config] of Object.entries(custom) as any){for(const options of [{},config]){
 const input=closeFunctions.has(name)?data.map((x:any)=>x.close):data;
 const fn=(api as any)['calc'+name];const expected=['SMA','EMA','WMA'].includes(name)?fn(input,options.period??5,options.decimals):fn(input,options);
 fixtures.push({name,dataset,options,expected});
}}}
const allOptions=Object.fromEntries(Object.keys(custom).filter(n=>!['SMA','EMA','WMA'].includes(n)).map(n=>[n.toLowerCase(),{}]));
const pipelines=[{dataset:'random',options:allOptions},{dataset:'holes',options:allOptions},{dataset:'random',options:{ma:{periods:[3,7],type:'wma'},macd:{short:3,long:7,signal:3}}},{dataset:'empty',options:allOptions},{dataset:'random',options:{}}].map(f=>({...f,expected:api.addIndicators(datasets[f.dataset],f.options),lookback:api.estimateIndicatorLookback(f.options),cumulative:api.hasCumulativeIndicator(f.options)}));
writeFileSync(new URL('../testdata/indicator-parity.json',import.meta.url),JSON.stringify({datasets,fixtures,pipelines})+'\n');
console.log(`Generated ${fixtures.length} indicator fixtures`);
