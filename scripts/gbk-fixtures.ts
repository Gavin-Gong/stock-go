// Optional maintainer oracle for all syntactically valid Windows-936/GBK pairs.
import {writeFileSync} from 'node:fs';
import {decodeGBK} from '../.upstream/src/core/parser';
const values:Record<string,string>={};
for(let b=0;b<=255;b++)values[b.toString(16).padStart(2,'0')]=decodeGBK(Uint8Array.of(b).buffer);
for(let a=0x81;a<=0xfe;a++)for(let b=0x40;b<=0xfe;b++){if(b===0x7f)continue;values[(a*256+b).toString(16)]=decodeGBK(Uint8Array.of(a,b).buffer)}
writeFileSync(new URL('../testdata/gbk-parity.json',import.meta.url),JSON.stringify(values)+'\n');console.log(`Generated ${Object.keys(values).length} GBK decoding cases`)
