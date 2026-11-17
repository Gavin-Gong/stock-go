// Optional deterministic upstream oracle; no network requests or JS runtime in Go tests.
import {writeFileSync} from 'node:fs';
import {CircuitBreaker} from '../.upstream/src/core/circuitBreaker';
import {RateLimiter} from '../.upstream/src/core/rateLimiter';
import {HostFallbackManager} from '../.upstream/src/core/fallback';
import {SdkError,HttpError} from '../.upstream/src/core/errors';
import {mergeProviderPolicy,resolveProviderPolicy,inferProviderFromUrl} from '../.upstream/src/core/providerPolicy';
const fixtures:any[]=[];const originalNow=Date.now;let now=1_000_000;Date.now=()=>now;
try{
 for(const [name,options,steps]of [
 ['recover',{failureThreshold:2,resetTimeout:10},['success','failure','failure','can','advance:10','state','can','can','success','state','failure','reset','state']],
 ['probeFail',{failureThreshold:1,resetTimeout:10},['failure','advance:10','can','failure','can','advance:10','can','release','can','success']],
 ['multiProbe',{failureThreshold:1,resetTimeout:10,halfOpenRequests:2},['failure','advance:10','can','can','can','success','can','success','state']],
 ['recycle',{failureThreshold:1,resetTimeout:10,probeRecycleTimeout:20},['failure','advance:10','can','advance:19','can','advance:1','can','success','state']],
 ['resetWhileOpen',{failureThreshold:1,resetTimeout:10},['failure','reset','can','success','state']],
 ]as any[]){now=1_000_000;const b=new CircuitBreaker(options);const expected=steps.map((step:string)=>{let allowed; if(step.startsWith('advance:'))now+=Number(step.split(':')[1]);else if(step==='can')allowed=b.canRequest();else if(step==='failure')b.recordFailure();else if(step==='success')b.recordSuccess();else if(step==='release')b.releaseProbe();else if(step==='reset')b.reset();return {allowed,...b.getStats()}});fixtures.push({kind:'breaker',name,options,steps,expected})}
 for(const options of [{requestsPerSecond:5,maxBurst:2},{requestsPerSecond:10,maxBurst:1},{requestsPerSecond:2.5,maxBurst:3}]){now=1_000_000;const r=new RateLimiter(options);const steps=['try','try','wait','advance:100','try','wait','advance:300','try','try','advance:5000','tokens'];const expected=steps.map(step=>{let acquired;if(step==='try')acquired=r.tryAcquire();if(step.startsWith('advance:'))now+=Number(step.split(':')[1]);return{acquired,tokens:r.getAvailableTokens(),wait:r.getWaitTime()}});fixtures.push({kind:'limiter',options,steps,expected})}
 for(const endpoint of ['https://push2his.eastmoney.com/path?type=1,2','https://33.push2his.eastmoney.com/path','https://17.push2.eastmoney.com/path','https://qt.gtimg.cn/path']){now=1_000_000;const h=new HostFallbackManager();const provider=inferProviderFromUrl(endpoint);const steps=['candidates','failure','candidates','advance:30000','candidates','success','candidates'];const expected=steps.map(step=>{if(step==='failure')h.recordFailure(endpoint,new SdkError({code:'NETWORK_ERROR'}));if(step==='success')h.recordSuccess(endpoint);if(step.startsWith('advance:'))now+=Number(step.split(':')[1]);return{urls:h.getCandidateUrls(endpoint,provider),stats:h.getStats().sort((a,b)=>a.host.localeCompare(b.host))}});fixtures.push({kind:'hosts',endpoint,provider,steps,expected})}
 for(const code of ['NETWORK_ERROR','TIMEOUT','PARSE_ERROR','ABORTED','UPSTREAM_ERROR','INVALID_ARGUMENT']){fixtures.push({kind:'fallback',code,expected:new HostFallbackManager().shouldFallback(new SdkError({code:code as any}))})}
 for(const status of [400,404,408,429,500,501,503])fixtures.push({kind:'fallback',status,expected:new HostFallbackManager().shouldFallback(new HttpError(status,''))});
 const base={timeout:2000,retry:{maxRetries:2,baseDelay:100,maxDelay:5000,backoffMultiplier:3},headers:{'X-Base':'yes'},userAgent:'base',rateLimit:{requestsPerSecond:5},circuitBreaker:{failureThreshold:2,resetTimeout:1000}};
 for(const override of [{},{timeout:500,retry:{maxRetries:0,retryOnTimeout:false},headers:{'X-Base':'override'},userAgent:'provider'},{rateLimit:{requestsPerSecond:10},circuitBreaker:{resetTimeout:40000,halfOpenRequests:2}},{headers:{'User-Agent':'explicit'},userAgent:'ignored',rotateUserAgent:true}]){const expected=resolveProviderPolicy(mergeProviderPolicy(base,override));fixtures.push({kind:'policy',base,override,expected})}
 writeFileSync(new URL('../testdata/governance-parity.json',import.meta.url),JSON.stringify(fixtures)+'\n');console.log(`Generated ${fixtures.length} governance fixtures`)
}finally{Date.now=originalNow}
