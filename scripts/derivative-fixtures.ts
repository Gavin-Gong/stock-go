// Optional oracle. Run with npx tsx scripts/derivative-fixtures.ts; Go tests read JSON only.
import {writeFileSync} from 'node:fs';
import * as domestic from '../.upstream/src/providers/eastmoney/futuresKline';
import * as global from '../.upstream/src/providers/eastmoney/futuresGlobal';
import * as inventory from '../.upstream/src/providers/eastmoney/futuresInventory';
import * as cffex from '../.upstream/src/providers/eastmoney/optionCffex';
import * as lhb from '../.upstream/src/providers/eastmoney/optionLhb';
import * as index from '../.upstream/src/providers/sina/optionIndex';
import * as commodity from '../.upstream/src/providers/sina/optionCommodity';
import * as etf from '../.upstream/src/providers/sina/optionEtf';
import {FUTURES_VARIETY_EXCHANGE,GLOBAL_FUTURES_MARKET,COMMODITY_OPTION_MAP} from '../.upstream/src/core/constants';
async function main(){const fixtures:any[]=[];
 async function record(operation:string,symbol:string,variant:string,responses:any[],call:(c:any)=>Promise<any>){const queries:any[]=[];let i=0;const request=async(url:string)=>{const parsed=new URL(url);parsed.searchParams.delete('callback');queries.push(Object.fromEntries(parsed.searchParams));return responses[Math.min(i++,responses.length-1)]};const client:any={get:request};globalThis.fetch=async(url:any)=>new Response(`/* <script> guarded </script> */ cb(${JSON.stringify(await request(String(url)))});`,{status:200});const expected=await call(client);fixtures.push({operation,symbol,variant,responses,queries,expected})}
 const history={data:{code:'contract',name:'测试合约',klines:['2024-01-02,10,11,12,9,100,200,3,4,5,6,777,888,999','2024-01-03,--,0,1.2abc,-1,0,--,--,0,-2,,0']}};
 for(const variety of Object.keys(FUTURES_VARIETY_EXCHANGE))await record('domestic',variety+'2605','daily',[history],c=>domestic.getFuturesHistoryKline(c,variety+'2605',{startDate:'20240101',endDate:'20240131'}));
 for(const symbol of ['RBM','IFM','M','TAM'])for(const period of ['weekly','monthly'])await record('domestic',symbol,period,[history],c=>domestic.getFuturesHistoryKline(c,symbol,{period:period as any,startDate:'20240101',endDate:'20240131'}));
 for(const variety of Object.keys(GLOBAL_FUTURES_MARKET))await record('global',variety+'00Y','daily',[history],c=>global.getGlobalFuturesKline(c,variety+'00Y',{startDate:'20240101',endDate:'20240131'}));
 await record('global','ZZ00Y','override',[{data:{klines:history.data.klines}}],c=>global.getGlobalFuturesKline(c,'ZZ00Y',{marketCode:999,startDate:'20240101',endDate:'20240131'}));
 const quote={dm:'GC00Y',name:'黄金',p:'123.5',zde:-2,zdf:'1.2abc',o:120,h:130,l:110,zjsj:125,vol:9,wp:5,np:4,ccl:200,cje:5000,xqj:2500,syr:4,rz:3};
 await record('spot','','pages',[{total:3,list:[quote]},{total:3,list:[{dm:'CL00Y',name:'原油',p:'--'}]},{total:3,list:[{}]}],c=>global.getGlobalFuturesSpot(c,{pageSize:2}));
 const dc={result:{pages:1,count:2,data:[{SECURITY_CODE:'CU',TRADE_CODE:'CU',TRADE_TYPE:'铜',TRADE_MARKET_CODE:'113',TRADE_DATE:'2024-01-02 00:00:00',ON_WARRANT_NUM:123,ADDCHANGE:'-2.5',REPORT_DATE:'2024-01-03T00:00:00',STORAGE_TON:'1.23',STORAGE_OUNCE:456},{}]}};
 await record('inventorySymbols','','',[dc],c=>inventory.getFuturesInventorySymbols(c));
 await record('inventory','cu','',[dc],c=>inventory.getFuturesInventory(c,'cu',{startDate:'20240101',pageSize:10}));
 for(const metal of ['gold','silver'])await record('comex',metal,'',[dc],c=>inventory.getComexInventory(c,metal as any,{pageSize:10}));
 await record('cffex','','',[{total:2,list:[quote,{}]}],c=>cffex.getCFFEXOptionQuotes(c));
 const row:any=Object.fromEntries(['TRADE_TYPE','TRADE_DATE','SECURITY_CODE','TARGET_NAME','MEMBER_NAME_ABBR','MEMBER_RANK','SELL_VOLUME','SELL_VOLUME_CHANGE','NET_SELL_VOLUME','SELL_VOLUME_RATIO','BUY_VOLUME','BUY_VOLUME_CHANGE','NET_BUY_VOLUME','BUY_VOLUME_RATIO','SELL_POSITION','SELL_POSITION_CHANGE','NET_SELL_POSITION','SELL_POSITION_RATIO','BUY_POSITION','BUY_POSITION_CHANGE','NET_BUY_POSITION','BUY_POSITION_RATIO'].map((k,i)=>[k,i+0.5]));Object.assign(row,{TRADE_DATE:'2024-01-02 00:00:00',SECURITY_CODE:'510050'});await record('lhb','510050','',[{result:{pages:1,data:[row,{}]}}],c=>lhb.getOptionLHB(c,'510050','2024-01-02'));
 const t={result:{status:{code:0},data:{up:[['1','2.5','3','4','5','6','-1','3000','io2612C3000'],['--','','0']],down:[['2','3','4','5','6','7','1','io2612P3000'],[]]}}};
 for(const product of ['ho','io','mo'])await record('indexSpot',product,product+'2612',[t],()=>index.getIndexOptionSpot(product as any,product+'2612'));
 for(const variety of Object.keys(COMMODITY_OPTION_MAP))await record('commoditySpot',variety,variety+'2612',[t],()=>commodity.getCommodityOptionSpot(variety,variety+'2612'));
 const daily=[{d:'2024-01-02',o:'1',h:'2',l:'0.5',c:'1.5',v:'123'},{d:'invalid',o:'--',h:null,l:'1x',c:0,v:0}];
 await record('indexKline','io2612C3000','',[daily],()=>index.getIndexOptionKline('io2612C3000'));
 await record('commodityKline','au2612C600','',[daily],()=>commodity.getCommodityOptionKline('au2612C600'));
 for(const cate of ['50ETF','300ETF','500ETF','科创50','科创板50'])await record('months',cate,'',[{result:{data:{contractMonth:['交易月份','2026-12','2027-01'],stockId:'510050',cateId:'50ETF',cateList:['50ETF','300ETF']}}}],()=>etf.getETFOptionMonths(cate as any));
 for(const variant of ['normal','expired']){const negative={result:{data:{remainderDays:variant==='stringNegative'?'-1':-1}}};const result={result:{data:{expireDay:'2026-12-23',remainderDays:36,stockId:'510050',other:{name:'50ETF'}}}};await record('expire','50ETF',variant,variant==='normal'?[result]:[negative,result],()=>etf.getETFOptionExpireDay('50ETF','2026-12'))}
 const minutes=[{i:'09:30',p:'1.5',v:'10',t:'20',a:'1.2',d:'2024-01-02'},{i:'09:31',p:'--',v:0},{i:'09:30',d:'2024-01-03'}];
 await record('minute','10009633','',[{result:{data:minutes}}],()=>etf.getETFOptionMinute('10009633'));
 await record('etfDaily','10009633','',[daily],()=>etf.getETFOptionDailyKline('10009633'));
 await record('fiveDay','10009633','',[{result:{data:[minutes,[{i:'09:32',p:'1.23'},{i:'09:33',d:'2024-01-04'}]]}}],()=>etf.getETFOption5DayMinute('10009633'));
 writeFileSync(new URL('../testdata/derivative-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} derivative fixtures`)
}main().catch(error=>{console.error(error);process.exitCode=1});
