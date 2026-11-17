// Pinned-source metadata and pure prompt templates. Go does not evaluate JavaScript.
import {writeFileSync} from 'node:fs';
import {TOOLS} from '../.upstream/src/mcp/tools';
import {PROMPT_SPECS} from '../.upstream/src/spec/prompts';
import {toPromptDef} from '../.upstream/src/spec/derive-prompt';
import {METHOD_SPECS} from '../.upstream/src/spec/methods';
const tools=TOOLS.map(({invoke,...t})=>({...t,path:METHOD_SPECS.find(s=>s.toolName===t.name)!.path.join('.')}));
const prompts=PROMPT_SPECS.map(s=>{const p=toPromptDef(s);const values=Object.fromEntries(s.args.map(a=>[a.name,`__STOCK_GO_ARG_${a.name}__`]));const {render,...meta}=p;return{...meta,args:s.args,template:render(values),emptyTemplate:s.name==='analyze_capital_flow'?render({}):undefined}});
writeFileSync(new URL('../mcp/tools.json',import.meta.url),JSON.stringify(tools,null,2)+'\n');writeFileSync(new URL('../mcp/prompts.json',import.meta.url),JSON.stringify(prompts,null,2)+'\n');
console.log(`${tools.length} tools (${tools.filter(t=>t.tier==='core').length} core), ${prompts.length} prompts (${prompts.filter(p=>p.tier==='core').length} core)`)
