// Audit actual public SDK namespace methods against the CLI/MCP spec and typed Go bindings.
import {readFileSync,writeFileSync} from 'node:fs';
import {StockSDK} from '../.upstream/src/sdk';
import * as cache from '../.upstream/src/core/cache';
import * as cacheStore from '../.upstream/src/cache';
import * as time from '../.upstream/src/core/time';
import * as parser from '../.upstream/src/core/parser';
import * as jsonp from '../.upstream/src/core/jsonp';
import * as jsvars from '../.upstream/src/core/jsVars';
import * as links from '../.upstream/src/externalLinks';
import * as utils from '../.upstream/src/core/utils';
import {METHOD_SPECS} from '../.upstream/src/spec/methods';
const sdk:any=new StockSDK();const source=readFileSync(new URL('../.upstream/src/sdk.ts',import.meta.url),'utf8');
const roots=[...source.matchAll(/\n  get (\w+)\(\)/g)].map(m=>m[1]);const actual:string[]=[];
function walk(value:any,path:string){const plain=Object.getPrototypeOf(value)===Object.prototype;for(const name of Object.keys(value)){const member=value[name];if(typeof member==='function')actual.push(`${path}.${name}`);else if(plain&&member&&typeof member==='object')walk(member,`${path}.${name}`)}if(!plain)for(const name of Object.getOwnPropertyNames(Object.getPrototypeOf(value))){if(name!=='constructor'&&typeof value[name]==='function')actual.push(`${path}.${name}`)}}
for(const root of roots)walk(sdk[root],root);actual.push('search');actual.sort();
const specs=METHOD_SPECS.map(s=>s.path.join('.')).sort();const go=[...readFileSync(new URL('../internal/operations/bindings.go',import.meta.url),'utf8').matchAll(/"([A-Za-z][A-Za-z.]+)":/g)].map(m=>m[1]).sort();
for(const [label,paths]of [['upstream namespaces',actual],['Go bindings',go]]as const)if(JSON.stringify(paths)!==JSON.stringify(specs))throw Error(`${label} mismatch: missing ${specs.filter(p=>!paths.includes(p))}, extra ${paths.filter(p=>!specs.includes(p))}`);
const publicHelperGroups=[
 {name:'cache',goPackage:'cache',mappings:[
  ['MemoryCache','New'],['MemoryCacheStore','NewMemoryStore'],['cacheThrough','CacheThrough'],['getSharedCache','GetShared'],['configureSharedCache','ConfigureShared'],['clearSharedCaches','ClearShared'],['getClientScopedCache','GetClientScoped'],['clearClientScopedCaches','ClearClientScoped'],['createCacheKey','CreateKey']]},
 {name:'time',goPackage:'.',mappings:[['MARKET_TZ','MarketTZCN'],['parseMarketTime','ParseMarketTime'],['buildTimeMeta','BuildTimeMeta'],['buildTimeMetaFromDateAndTime','BuildTimeMetaFromDateAndTime'],['formatInTz','FormatInTZ']]},
 {name:'parsing',goPackage:'.',mappings:[['decodeGBK','DecodeGBK'],['parseResponse','ParseResponse'],['safeNumber','SafeNumber'],['safeNumberOrNull','SafeNumberOrNull'],['toNumber','ToNumber'],['toNumberSafe','ToNumberSafe'],['toFiniteNumberOrNull','ToFiniteNumberOrNull'],['extractJsonFromJsonp','ExtractJSONFromJSONP'],['jsonpRequest','JSONPRequest'],['parseJsVars','ParseJSVars'],['fetchJsVars','FetchJSVars']]},
 {name:'externalLinks',goPackage:'.',mappings:[['generateSearchExternalLinks','GenerateSearchExternalLinks']]},
 {name:'utils',goPackage:'.',mappings:[['chunkArray','ChunkArray'],['asyncPool','AsyncPool']]},
];
const modules:any={cache:{...cache,...cacheStore},time,parsing:{...parser,...jsonp,...jsvars},externalLinks:links,utils};
for(const group of publicHelperGroups)for(const [name]of group.mappings)if(!(name in modules[group.name]))throw Error(`Missing upstream helper ${group.name}.${name}`);
const privateMethods=[...source.matchAll(/^  private (?:async )?(\w+)[<(]/gm)].map(m=>m[1]);
const lifecycle=Object.getOwnPropertyNames(StockSDK.prototype).filter(name=>name!=='constructor'&&name!=='search'&&!privateMethods.includes(name)&&typeof Object.getOwnPropertyDescriptor(StockSDK.prototype,name)?.value==='function').sort();
if(JSON.stringify(lifecycle)!==JSON.stringify(['clearCaches']))throw Error(`Unmapped SDK lifecycle methods: ${lifecycle}`);
const lifecycleMethods=[{source:'clearCaches',go:'SDK.ClearCaches'}];
const audit={lifecycleMethods,baseline:'c6c9a790870c5c607379e542e214a02ea05979c4',sourceMethods:actual,goBindings:go,cliMethods:specs,mcpTools:METHOD_SPECS.filter(s=>s.mcp!==false).map(s=>({name:s.toolName,path:s.path.join('.')})),publicHelperGroups,pendingPublicHelpers:[],platformDifferences:['browser-only script injection/mutex replaced by Go HTTP and context','indicator context/registry objects remain internal; public Compute/Lookback are available']};
writeFileSync(new URL('../testdata/api-audit.json',import.meta.url),JSON.stringify(audit,null,2)+'\n');console.log(`API audit PASS: ${actual.length} actual namespace methods = ${specs.length} CLI methods = ${go.length} typed Go bindings; ${audit.mcpTools.length} MCP tools`);console.log(`${publicHelperGroups.length} public helper groups mapped (${publicHelperGroups.reduce((n,g)=>n+g.mappings.length,0)} entries), ${audit.pendingPublicHelpers.length} remain`)
