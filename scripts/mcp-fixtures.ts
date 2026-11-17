// Optional pinned-source MCP oracle. Go tests consume JSON without JS execution.
import {writeFileSync} from 'node:fs';
import {TOOLS} from '../.upstream/src/mcp/tools';
import {PROMPTS} from '../.upstream/src/mcp/prompts';
import {METHOD_SPECS} from '../.upstream/src/spec/methods';
import {toToolResult,toolErrorResult} from '../.upstream/src/mcp/result';
import {negotiateProtocolVersion} from '../.upstream/src/mcp/protocol';
import {dispatchMessage} from '../.upstream/src/mcp/server';
import {SdkError} from '../.upstream/src/core/errors';
async function main(){const fixtures:any[]=[];
 fixtures.push({kind:'tools',expected:TOOLS.map(({name,description,inputSchema})=>({name,description,inputSchema}))});fixtures.push({kind:'prompts',expected:PROMPTS.map(({name,title,description,arguments:args})=>({name,title,description,arguments:args}))});
 for(const t of TOOLS){const args:any={};for(const [key,p]of Object.entries(t.inputSchema.properties)){args[key]=p.default??p.enum?.[0]??(p.type==='string'?({symbol:'600519',code:'110011',keyword:'茅台',startDate:'20240101',endDate:'20240131',date:'20240102',month:'2026-12',contract:'io2612',variety:'au',themeCode:'BK0474'}as any)[key]??'value':p.type==='array'?['600519','000001']:p.type==='integer'?2:p.type==='boolean'?false:p.type==='object'?{ma:{periods:[5,10]},macd:true}:null)}
  const sdk:any={};const calls:any[]=[];for(const spec of METHOD_SPECS){let obj=sdk;for(const part of spec.path.slice(0,-1))obj=obj[part]??={};obj[spec.path.at(-1)!]=(...a:any[])=>{calls.push({path:spec.path.join('.'),args:a});return []}}
  const expected=await t.invoke(sdk,args);fixtures.push({kind:'call',name:t.name,args,calls,expected:toToolResult(expected)});
 }
 const ctx:any={sdk:{},tools:TOOLS,toolMap:new Map(TOOLS.map(t=>[t.name,t])),prompts:PROMPTS,promptMap:new Map(PROMPTS.map(p=>[p.name,p]))};
 for(const p of PROMPTS){const required=Object.fromEntries(p.arguments.filter(a=>a.required).map(a=>[a.name,'测试600519']));const all=Object.fromEntries(p.arguments.map(a=>[a.name,a.name==='period'?'weekly':'value ${symbol} __STOCK_GO_ARG_period__']));for(const args of [required,all,{},Object.fromEntries(p.arguments.map(a=>[a.name,0])),Object.fromEntries(p.arguments.map(a=>[a.name,false]))]){const msg:any={jsonrpc:'2.0',id:'p',method:'prompts/get',params:{name:p.name,arguments:args}};fixtures.push({kind:'prompt',message:msg,expected:await dispatchMessage(msg,ctx)})}}
 for(const value of ['2025-11-25','2025-06-18','2024-11-05','unknown',null,42])fixtures.push({kind:'version',value,expected:negotiateProtocolVersion(value)});
 for(const value of [null,[],[1,2],Array.from({length:200},(_,i)=>i),Array.from({length:201},(_,i)=>({i})),{b:1,a:2},'a'.repeat(80001),'中'.repeat(79999),'😀'.repeat(40000),{nested:'z'.repeat(80000)}])fixtures.push({kind:'result',value,expected:toToolResult(value)});
 for(const options of [{code:'NETWORK_ERROR',message:'offline',provider:'eastmoney',url:'https://example.test'},{code:'HTTP_ERROR',message:'not found',status:404},{code:'INVALID_ARGUMENT',message:'bad argument'}])fixtures.push({kind:'error',options,expected:toolErrorResult(new SdkError(options as any))});
 for(const message of [{jsonrpc:'2.0',id:0,method:'ping'},{jsonrpc:'1.0',id:1,method:'ping'},{jsonrpc:'2.0',id:'s',method:'unknown'},{jsonrpc:'2.0',method:'unknown'},{jsonrpc:'2.0',method:'notifications/initialized'},{jsonrpc:'2.0',id:2,method:'tools/call',params:[]},{jsonrpc:'2.0',id:3,method:'tools/call',params:{name:'missing'}},{jsonrpc:'2.0',id:4,method:'prompts/get',params:{name:'missing'}},{jsonrpc:'2.0',id:5,method:'tools/call',params:{name:'search',arguments:[]}}])fixtures.push({kind:'rpc',message,expected:await dispatchMessage(message as any,ctx)});
 writeFileSync(new URL('../testdata/mcp-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} MCP fixtures`)
}main().catch(error=>{console.error(error);process.exitCode=1});
