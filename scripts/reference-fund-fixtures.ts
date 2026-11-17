// Optional generator: npx tsx scripts/reference-fund-fixtures.ts
import {writeFileSync} from 'node:fs';
import * as north from '../.upstream/src/providers/eastmoney/northbound';
import {getDividendDetail} from '../.upstream/src/providers/eastmoney/dividend';
import * as fund from '../.upstream/src/providers/eastmoney/fund';
import * as theme from '../.upstream/src/providers/eastmoney/fundTheme';
import {TradingCalendarService} from '../.upstream/src/sdk/tradingCalendarService';
async function main(){const fixtures:any[]=[];
 const dates=['2024-01-02','2024-01-03','2024-01-05'];const calendar=new TradingCalendarService({getTradingCalendar:async()=>dates} as any);
 for(const date of ['20240102','2024-01-04'])for(const operation of ['isTradingDay','nextTradingDay','prevTradingDay']){
  if(operation==='prevTradingDay'&&date==='20240102')continue;
  fixtures.push({kind:'calendar',operation,date,calendar:dates,expected:await (calendar as any)[operation](date)});
 }
 for(const market of ['A','HK','US'])for(const at of ['2024-07-09T00:00:00Z','2024-07-09T01:30:00Z','2024-07-09T03:30:00Z','2024-07-09T04:30:00Z','2024-07-09T05:00:00Z','2024-07-09T13:30:00Z','2024-07-09T20:00:00Z','2024-07-13T14:00:00Z'])fixtures.push({kind:'calendar',operation:'marketStatus',market,at,expected:calendar.getMarketStatus(market as any,new Date(at))});
 const api=Object.fromEntries(['TRADE_DATE','MUTUAL_TYPE','MUTUAL_TYPE_NAME','FUNDS_DIRECTION','status','netBuyAmt','dayNetAmtIn','dayAmtRemain','f104','f106','f105','INDEX_CODE','INDEX_NAME','INDEX_f3','SECURITY_CODE','SECURITY_NAME','SECURITY_NAME_ABBR','CLOSE_PRICE','CHANGE_RATE','HOLD_SHARES','HOLD_MARKET_CAP','HOLD_RATIO','A_SHARES_RATIO','ADD_SHARES','ADD_MARKET_CAP','ADD_MARKET_CAP_PROPORTION','BOARD_NAME','NET_DEAL_AMT','BUY_AMT','SELL_AMT','ACCUM_DEAL_AMT','FUND_INFLOW','QUOTA_BALANCE','LEAD_STOCKS_CODE','LEAD_STOCKS_NAME','LS_CHANGE_RATE','REPORT_DATE','PLAN_NOTICE_DATE','BONUS_IT_RATIO','BONUS_RATIO','IT_RATIO','PRETAX_BONUS_RMB','IMPL_PLAN_PROFILE','DIVIDENT_RATIO','BASIC_EPS','BVPS','PER_CAPITAL_RESERVE','PER_UNASSIGN_PROFIT','PNP_YOY_RATIO','TOTAL_SHARES','EQUITY_RECORD_DATE','EX_DIVIDEND_DATE','PAY_DATE','ASSIGN_PROGRESS','NOTICE_DATE'].map((k,i)=>[k,i+0.5]));
 Object.assign(api,{TRADE_DATE:'2024-01-02 00:00:00',SECURITY_CODE:'600519',SECURITY_NAME:'贵州茅台',SECURITY_NAME_ABBR:'茅台',REPORT_DATE:'2023-12-31',PLAN_NOTICE_DATE:'2024-01-05 00:00:00',PUBLISH_DATE:null,EQUITY_RECORD_DATE:'2024-01-06',EX_DIVIDEND_DATE:'bad',PAY_DATE:null,NOTICE_DATE:'2024-01-03',IMPL_PLAN_PROFILE:'10派1元',ASSIGN_PROGRESS:'实施'});
 const response={result:{pages:1,count:2,data:[api,{SECURITY_CODE:'000001',TRADE_DATE:'invalid',PRETAX_BONUS_RMB:null}]}};
 for(const operation of ['summary','holdingRank','historyNorth','historySouth','individual','dividend']){const client:any={get:async()=>response};let expected;
  if(operation==='summary')expected=await north.getNorthboundFlowSummary(client);
  if(operation==='holdingRank')expected=await north.getNorthboundHoldingRank(client,{market:'shanghai',period:'month',date:'2024-01-02'});
  if(operation==='historyNorth'||operation==='historySouth')expected=await north.getNorthboundHistory(client,operation==='historyNorth'?'north':'south',{startDate:'20240101',endDate:'20240131'});
  if(operation==='individual')expected=await north.getNorthboundIndividual(client,'600519.SH',{startDate:'20240101',endDate:'20240131'});
  if(operation==='dividend')expected=await getDividendDetail(client,'600519.SH');
  fixtures.push({kind:'dc',operation,response,expected});
 }
 const minutes={data:{s2n:['09:30,1,2,3,4,5','09:31,--,2,3,4,--'],s2nDate:'20240102',n2s:['09:30,6,2,7,4,8'],n2sDate:'2024-01-03'}};
 for(const direction of ['north','south'])fixtures.push({kind:'northMinute',operation:direction,response:minutes,expected:await north.getNorthboundMinute({get:async()=>minutes} as any,direction as any)});
 const ts=1704124800000;
 const vars:any={fS_code:'110011',fS_name:'测试基金',fund_sourceRate:'1.5',fund_Rate:'0.15',fund_minsg:'10',stockCodesNew:['1.600519',' 0.000001 ','bare',''],zqCodesNew:'1.019674,0.123001',Data_netWorthTrend:[{x:ts,y:1.5,equityReturn:'0.5',unitMoney:''},{y:99},{x:'bad',y:1},{x:ts+86400000,y:'1.2x',equityReturn:'--'}],Data_ACWorthTrend:[[ts,'2.5abc'],[ts+86400000,'2.6'],['bad',9]],Data_rateInSimilarType:[{x:ts,y:'12',sc:'100'},{x:null,y:1},{x:ts+86400000,y:'--',sc:110}],Data_rateInSimilarPersent:[[ts,88.5],[ts+86400000,'9x']],Data_assetAllocation:{categories:['2024-01-01','bad'],series:[{name:'股票占净比',data:[90,null]},{name:'现金占净比',data:[10]}]},Data_fundSharesPositions:[[ts,88],['bad',90]],Data_currentFundManager:[{id:'M1',name:'经理',pic:' http://image ',star:5,workTime:' 3年 ',fundSize:' 10亿 ',power:{avr:'80',categories:['选股'],data:[70],dsc:['好']}},null],Data_performanceEvaluation:{avr:'90',categories:['收益'],data:[95]},Data_holderStructure:{categories:['2024-01-01'],series:[{name:'机构持有比例',data:[40]},{name:'个人持有比例',data:[60]}]},Data_fluctuationScale:{categories:['2024-01-01','2024-04-01'],series:[{y:12.5,mom:'3.5%'}]},Data_buySedemption:{categories:['2024-01-01'],series:[{name:'期间申购',data:[1.5]},{name:'总份额',data:[12.25]}]},syl_1y:'1.5',syl_3y:'--',syl_6y:'1.2x',syl_1n:'10',swithSameType:[['110011_基金_名字_1.2','bad','110022_基金B'],[],['_empty']]};
 for(const variant of ['full','partial']){const text=Object.entries(variant==='full'?vars:{fS_name:'部分数据'}).map(([k,v])=>`var ${k} = ${JSON.stringify(v)};`).join('\n');const client:any={get:async()=>text};for(const operation of ['nav','rank','profile']){const expected=operation==='nav'?await fund.getFundNavHistory(client,'110011'):operation==='rank'?await fund.getFundRankHistory(client,'110011'):await fund.getFundProfile(client,'110011');fixtures.push({kind:'fund',operation,variant,text,expected})}}
 const dividendText=(page:number)=>`var pageinfo=[2,1,${page}]; var jjfh_data=${JSON.stringify([[page===1?'110011':'000001','测试基金','2024-01-02','2024-01-03',page===1?'0.5':'--','2024-01-05',' 派现 ']])};`;
 for(const all of [false,true]){const client:any={get:async(url:string)=>dividendText(Number(new URL(url).searchParams.get('page')))};const expected=await fund.getFundDividendList(client,{year:2024,page:all?'all':1,code:'110011'});fixtures.push({kind:'fundDividend',operation:all?'all':'page',texts:[dividendText(1),dividendText(2)],expected})}
 for(const empty of [false,true]){
  const list=empty?{Data:[],pageinfo:{}}:{Data:[{code:'T1',name:'主题',ZDF:'1.5',SYL_W:'--',SYL_M:'2x',isIndustry:0},{code:'T2',name:'行业',isIndustry:1}],pageinfo:{totalPages:2,page:1,pageSize:20}};
  fixtures.push({kind:'theme',operation:'list',response:list,expected:await theme.getThemeList({get:async()=>list} as any)});
  const funds=empty?{Data:[],fundinfo:{}}:{Data:[{fscode:'110011',abname:'基金',ftype:'混合型',ZDF:'--',SYL_Z:'1.5',SYL_Y:'2',SYL_3Y:'3',SYL_1N:'4',dwjz:'1.2'}],fundinfo:{total:1,pageindex:1,pagesize:20}};
  fixtures.push({kind:'theme',operation:'funds',response:funds,expected:await theme.getThemeFunds({get:async()=>funds} as any,'T1')});
 }
 writeFileSync(new URL('../testdata/reference-fund-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} reference/calendar/northbound/fund fixtures`);
}
main().catch(error=>{console.error(error);process.exitCode=1});
