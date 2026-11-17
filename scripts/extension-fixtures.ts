// Optional generator. Go tests use JSON; no JS/provider runtime is needed.
import {writeFileSync} from 'node:fs';
import * as industry from '../.upstream/src/providers/eastmoney/industryBoard';
import * as concept from '../.upstream/src/providers/eastmoney/conceptBoard';
import * as flow from '../.upstream/src/providers/eastmoney/fundFlow';
async function main(){
 const row:any=Object.fromEntries(Array.from({length:300},(_,i)=>[`f${i+1}`,i+1.25]));Object.assign(row,{f12:'600519',f14:'贵州茅台',f57:'600519',f58:'贵州茅台',f86:1704159000,f128:'领涨股',f204:'000001',f205:'龙头股'});
 const rows=[row,{f12:'000001',f14:'平安银行',f2:'--',f3:-1.5,f4:'2.5%',f128:''}];
 const csv='2024-01-02,10,11,12,9,100,200,3,4,5,--';const trend='2024-01-02 09:30,10,11,12,9,100,200,11';
 const fixtures:any[]=[];
 for(const [kind,api,prefix]of [['industry',industry,'Industry'],['concept',concept,'Concept']]as any){for(const operation of ['List','Spot','Constituents','Kline','Minute1','Minute5']){
  const responses:any=operation==='Spot'?{data:{f43:12345,f44:'555',f45:12000,f46:null,f47:100,f48:123456,f170:-123,f171:456,f168:789,f169:30}}:operation==='List'||operation==='Constituents'?{data:{total:rows.length,diff:rows}}:operation==='Minute1'?{data:{trends:[trend]}}:{data:{klines:[csv]}};
  const client:any={get:async()=>responses};let expected:any;
  if(operation==='List')expected=await api[`get${prefix}List`](client);
  else if(operation==='Minute1'||operation==='Minute5')expected=await api[`get${prefix}MinuteKline`](client,'BK0475',{period:operation==='Minute1'?'1':'5'});
  else expected=await api[`get${prefix}${operation}`](client,'BK0475');
  fixtures.push({kind,operation,response:responses,expected});
 }}
 for(const operation of ['individual','market','sectorHistory','realtime']){
  const response=operation==='realtime'?{data:row}:{data:{klines:[csv]}};const client:any={get:async()=>response};let expected:any;
  if(operation==='individual')expected=await flow.getIndividualFundFlow(client,'600519');
  if(operation==='market')expected=await flow.getMarketFundFlow(client);
  if(operation==='sectorHistory')expected=await flow.getSectorFundFlowHistory(client,'BK0475');
  if(operation==='realtime')expected=await flow.getRealtimeFundFlow(client,['600519','sh600519','930955']);
  fixtures.push({kind:'flow',operation,response,expected});
 }
 for(const indicator of ['today','3day','5day','10day'])for(const operation of ['rank','sectorRank']){
  const response={data:{total:rows.length,diff:rows}};const client:any={get:async()=>response};const options={indicator,sectorType:'concept'};
  const expected=operation==='rank'?await flow.getFundFlowRank(client,options as any):await flow.getSectorFundFlowRank(client,options as any);
  fixtures.push({kind:'flow',operation,response,options,expected});
 }
 writeFileSync(new URL('../testdata/extension-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} board/fund-flow fixtures`);
}
main().catch(error=>{console.error(error);process.exitCode=1});
