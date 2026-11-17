// Optional original CLI oracle. Go uses fixture JSON without Node or the upstream runtime.
import {writeFileSync} from 'node:fs';
import {NAMESPACE_COMMANDS,ALIAS_COMMANDS,findCommand,collectBooleanFlags} from '../.upstream/src/cli/manifest';
import {dispatch} from '../.upstream/src/cli/dispatch';
import {parseArgv} from '../.upstream/src/cli/parser';
import {formatOutput} from '../.upstream/src/cli/format';
async function main(){const fixtures:any[]=[];
 const sample=(name:string)=>({symbol:'600519',code:'110011',keyword:'茅台',params:'sh600519',direction:'south',market:'HK',type:'zt',date:'20240102',period:'3month',product:'io',contract:'io2612',cate:'50ETF',month:'2026-12'}[name]??'value');
 const value=(opt:any)=>opt.type==='enum'?opt.enum[0]:opt.type==='boolean'?true:opt.type==='number[]'?'5,10':opt.type==='number'?'2':opt.flag==='start'?'20240101':opt.flag==='end'?'20240131':opt.flag==='date'?'20240102':'value';
 async function record(tokens:string[],options:any){const match=findCommand(tokens);if(!match)throw Error(tokens.join(' '));const calls:any[]=[];const sdk:any={};for(const cmd of NAMESPACE_COMMANDS){let obj=sdk;for(const key of cmd.path.slice(0,-1))obj=obj[key]??={};obj[cmd.path.at(-1)!]=(...args:any[])=>{calls.push({path:cmd.path.join('.'),args});return Promise.resolve([])}};const expected=await dispatch(sdk,match.spec,{positional:match.rest,options:{...options}});calls.sort((a,b)=>a.path.localeCompare(b.path));fixtures.push({kind:'dispatch',tokens,options,calls,expected})}
 for(const cmd of NAMESPACE_COMMANDS){const codes=cmd.argShape==='codes[]'||cmd.argShape==='codes+options';const pos=codes?['600519','000001']:(cmd.positional??[]).map(p=>p.enum?.[0]??sample(p.name));const opts=Object.fromEntries((cmd.options??[]).map(o=>[o.flag,value(o)]));await record([...cmd.path,...pos],opts)}
 for(const [tokens,opts]of [
 [['quote','600519','00700','AAPL'],{}], [['quote','600519'],{full:true}], [['quote','110011'],{market:'fund'}], [['kline','AAPL'],{period:'weekly',adjust:'none',limit:'2'}], [['minute','00700'],{period:'1',ndays:'3'}], [['indicators','600519'],{ma:['5','10,20'],rsi:true,macd:true,boll:'false',cci:'yes'}], [['codes','a'],{simple:true,board:'kc'}], [['codes','us'],{simple:true}], [['status','us'],{}], [['ztpool','strong'],{date:'20240102'}], [['call','search'],{args:'["茅台"]'}], [['northbound','history','south'],{direction:'north',start:'20240101'}], [['search','中文 空格'],{limit:'1'}]
 ]as any[])await record(tokens,opts);
 for(const args of [[],['quote','600519','--full'],['--pretty','quote','600519'],['quote','600519','--market=hk','-f','csv'],['indicators','--macd','600519','--ma','5','--ma=10,20'],['call','search','--args','["-text"]'],['--limit','-5','--','--x','-3.14'],['-f=json','--timeout','1000'],['--x','a','--x','b','--x'],['--x','--y','-'],['-3.14','-','--x=-2'],['--triggered=false','--pageSize','20']])fixtures.push({kind:'parse',args,expected:parseArgv(args,collectBooleanFlags())});
 const values=[null,[],42,'文本',[1,null,'a,b'],[{code:'600519',name:'贵州茅台',price:1.5},{code:'000001',name:'平安银行',price:null,extra:'换行\n"引号"'}],{nested:{b:2,a:1},array:[1,2],empty:null},[{},{}],[{name:'中文',n:'  '},{name:'English',n:'0x10'}],[{emoji:'😀',n:-2}]];
 for(const value of values)for(const format of ['json','csv','table'])for(const pretty of format==='json'?[false,true]:[false])fixtures.push({kind:'format',body:JSON.stringify(value),format,pretty,expected:formatOutput(value,format as any,pretty)});
 writeFileSync(new URL('../testdata/cli-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} CLI fixtures`)
}main().catch(error=>{console.error(error);process.exitCode=1});
