// Maintainer-only parity generator; Go tests read checked-in JSON without Node.js.
// Run: npx tsx scripts/kline-fixtures.ts (requires pinned .upstream checkout).
import {writeFileSync} from 'node:fs';
import {parseEmKlineCsv} from '../.upstream/src/providers/eastmoney/utils';
import {getTencentHistoryKline,getTencentMinuteKline} from '../.upstream/src/providers/tencent/kline';
import {getSinaHistoryKline} from '../.upstream/src/providers/sina/kline';
async function main(){
 const fixtures:any[]=[];
 for(const line of ['2024-01-02,10,11,12,9,100,200,3,4,5,6','2024-01-02,--,,12,-,100,200,--,4,,6','2024-01-02,10.5%,11e1,12,9,100']) fixtures.push({kind:'csv',line,expected:parseEmKlineCsv(line)});
 const history={code:0,data:{sh600519:{hfqday:[['2024-01-01','100','100','110','90','10'],['2024-01-02','110','120','130','100','20'],['2024-01-03','130','200','210','120','30']]}}};
 const anchor={data:{sh600519:{day:[['2024-01-03','13','20','21','12','30']]}}};
 const options={startDate:'20240102',endDate:'20240103'};
 const client={get:async (url:string)=>new URL(url).searchParams.get('param')?.endsWith(',hfq')?history:anchor} as any;
 fixtures.push({kind:'tencentHistory',history,anchor,options,expected:await getTencentHistoryKline(client,'600519',options)});
 const minute={data:{sh600519:{m5:[['202401020930','10','11','12','9','20'],['202401020935','11','12','13','10','30']]}}};
 fixtures.push({kind:'tencentMinute',minute,options:{period:'5',startDate:'20240102',endDate:'20240102'},expected:await getTencentMinuteKline({get:async()=>minute} as any,'600519',{period:'5',startDate:'20240102',endDate:'20240102'})});
 const sina=[{day:'2023-12-29',open:'8',close:'10',high:'11',low:'7',volume:'1000'},{day:'2024-01-02',open:'10',close:'12',high:'13',low:'9',volume:'2000'},{day:'2024-01-03',open:'12',close:'14',high:'15',low:'11',volume:'3000'}];
 const weekly={period:'weekly' as const,startDate:'20240101',endDate:'20240131'};
 fixtures.push({kind:'sinaHistory',sina,options:weekly,expected:await getSinaHistoryKline({get:async()=>sina} as any,'600519',weekly)});
 writeFileSync(new URL('../testdata/kline-parity.json',import.meta.url),JSON.stringify(fixtures,null,2)+'\n');console.log(`Generated ${fixtures.length} upstream K-line fixtures`);
}
main().catch(error=>{console.error(error);process.exitCode=1});
