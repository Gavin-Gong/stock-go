// Optional generator: npx tsx scripts/chip-fixtures.ts. Go tests need no JS runtime.
import {writeFileSync} from 'node:fs';
import {calcChipDistribution} from '../.upstream/src/indicators';
const bars=Array.from({length:70},(_,i)=>{const close=Math.round((30+Math.sin(i/5)*3+i/20)*10000)/10000;return{date:`D${i}`,open:close-0.2,close,high:close+1,low:close-1,turnoverRate:0.5+(i%9)}});
const datasets:any={trend:bars,flat:bars.slice(0,12).map(b=>({...b,open:10,close:10,high:10,low:10})),holes:bars.map((b,i)=>i%7===0?{...b,close:null,turnoverRate:null}:b),zeroTurnover:bars.slice(0,12).map(b=>({...b,turnoverRate:null})),negative:bars.slice(0,12).map(b=>({...b,open:-2,high:-1,low:-3,close:-2,turnoverRate:-1})),negativePrice:bars.slice(0,12).map((b,i)=>({...b,open:-3+i/10,high:-1+i/10,low:-4+i/10,close:-2+i/10})),dirty:bars.slice(0,12).map((b,i)=>({...b,open:i%2===0?1e6:-1e6,close:i%2===0?-1e6:1e6,turnoverRate:130})),empty:[]};
const configs=[{}, {range:0}, {range:3,tail:5}, {range:1,includeHistogram:'last',tail:3}, {range:5,includeHistogram:'all',tail:5,decimals:6},{range:0,tail:0}];
const fixtures:any[]=[];for(const [dataset,data]of Object.entries(datasets) as any){for(const options of configs){fixtures.push({dataset,options,expected:calcChipDistribution(data,options as any)})}}
writeFileSync(new URL('../testdata/chip-parity.json',import.meta.url),JSON.stringify({datasets,fixtures})+'\n');console.log(`Generated ${fixtures.length} chip fixtures`);
