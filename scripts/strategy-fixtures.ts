// Optional parity generator. Go tests use only checked-in JSON.
import {writeFileSync} from 'node:fs';
import {addIndicators} from '../.upstream/src/indicators';
import {calcSignals} from '../.upstream/src/signals';
import {backtest,screen} from '../.upstream/src/screener';
const bars=Array.from({length:180},(_,i)=>{const close=50+Math.sin(i*0.3)*10+i/25;return{date:`bar-${i}`,timestamp:i%17===0?null:1700000000000+i*86400000,open:close-0.5,close,high:close+1,low:close-1,volume:100+i}});
const enriched=addIndicators(bars,{ma:{periods:[3,8]},macd:true,kdj:true,rsi:{periods:[6]},boll:true,sar:true});
const signalOptions={ma:{fast:3,slow:8},macd:true,kdj:{},rsi:{period:6},boll:true,sar:true};
const signals=[{bars:enriched,options:signalOptions,expected:calcSignals(enriched,signalOptions)},{bars:enriched,options:{},expected:calcSignals(enriched,{})},{bars:enriched.map(x=>({...x,timestamp:null})),options:signalOptions,expected:calcSignals(enriched.map(x=>({...x,timestamp:null})),signalOptions)}];
const scenarios=[
 {name:'basic',prices:[10,12,15],actions:['buy','hold','sell']},
 {name:'fees',prices:[10,12,15],actions:['buy','hold','sell'],fee:{buy:0.03,sell:0.07}},
 {name:'fraction',prices:[10,12,8,15,16],actions:['buy','hold','sell','buy','hold'],positionSize:0.4,initialCapital:1000,fee:0.01},
 {name:'forced',prices:[10,12,null,0],actions:['buy','hold','hold','hold'],fee:0.01},
 {name:'pending-sell',prices:[10,12,null,0],actions:['buy','hold','sell','hold'],fee:{buy:0.01,sell:0.03}},
 {name:'pending-buy',prices:[null,0,10,12],actions:['buy','hold','hold','sell']},
 {name:'latest-intent',prices:[null,0,10,12],actions:['buy','sell','hold','hold']},
 {name:'holes',prices:[null,10,12,null,14],actions:['buy','buy','hold','sell','sell'],holes:[0,3]},
 {name:'all-invalid',prices:[null,0,-1],actions:['buy','sell','hold']},
 {name:'empty',prices:[],actions:[]},
 {name:'zero-return',prices:[10,10],actions:['buy','sell']},
 {name:'fees-drawdown',prices:[10,10,10],actions:['buy','hold','hold'],fee:0.1},
];
const backtests=scenarios.map((c:any)=>{const data=c.prices.map((close:any,i:number)=>c.holes?.includes(i)?null:{close,date:`D${i}`});const options:any={klines:data,strategy:(_:any,i:number)=>c.actions[i],getDate:(b:any)=>b.date};for(const k of ['fee','positionSize','initialCapital'])if(c[k]!==undefined)options[k]=c[k];return{...c,bars:data,expected:backtest(options)}});
const items=[{id:'a',v:'12'},{id:'b',v:null},{id:'c',v:'2'},{id:'d',v:'NaN'},{id:'e',v:'12'},{id:'f',v:' '},{id:'g',v:100},{id:'h',v:'1.5%'},{id:'i',v:'0x10'}];
const screens=['asc','desc'].map(direction=>({items,direction,expected:screen(items).sortBy(x=>x.v,direction as any).toArray()}));
writeFileSync(new URL('../testdata/strategy-parity.json',import.meta.url),JSON.stringify({signals,backtests,screens})+'\n');
console.log(`Generated ${signals.length} signal, ${backtests.length} backtest and ${screens.length} screener fixtures`);
