// Optional maintainer tool. Run: npx tsx scripts/quote-fixtures.ts
// Requires the pinned upstream checkout in .upstream; Go tests do not require Node.js.
import { writeFileSync } from 'node:fs';
import { parseFullQuote,parseSimpleQuote,parseHKQuote,parseUSQuote,parseFundQuote,parsePanelLargeOrder } from '../.upstream/src/providers/tencent/parsers';
const parsers={FullQuote:parseFullQuote,SimpleQuote:parseSimpleQuote,HKQuote:parseHKQuote,USQuote:parseUSQuote,FundQuote:parseFundQuote,PanelLargeOrder:parsePanelLargeOrder};
const fixtures:unknown[]=[];
for(const [kind,parse] of Object.entries(parsers)){
 for(const length of kind==='FundQuote'?[9]:kind==='SimpleQuote'?[8,11]:kind==='PanelLargeOrder'?[4]:[38,50,74]){
  for(const variant of ['full','placeholders','invalid-time']){
   const fields=Array.from({length},(_,i)=>String(i+0.125));
   fields[0]='1';fields[1]='贵州茅台';fields[2]=kind==='HKQuote'?'HSI':kind==='USQuote'?'.DJI':'600519';
   if(kind==='FundQuote'){fields[0]='110011';fields[8]='2024-07-09'}
   if(length>30)fields[30]='20240709153000';
   if(kind==='HKQuote'&&length>=50)fields[length-3]='HKD';
   if(variant==='placeholders'){for(let i=3;i<length;i++){if(i%3===0)fields[i]='--';else if(i%3===1)fields[i]='';else fields[i]='-1.5%'}}
   if(variant==='invalid-time'){if(length>30)fields[30]='bad';if(kind==='FundQuote')fields[8]='bad'}
   fixtures.push({kind,label:`${kind}/${length}/${variant}`,fields,expected:parse(fields)});
  }
 }
}
writeFileSync(new URL('../testdata/quote-parity.json',import.meta.url),JSON.stringify(fixtures,null,2)+'\n');
console.log(`Generated ${fixtures.length} fixtures from upstream c6c9a790870c5c607379e542e214a02ea05979c4`);
