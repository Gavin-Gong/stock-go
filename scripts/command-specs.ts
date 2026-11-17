// Maintainer-only generation from the pinned upstream; embedded JSON needs no Node runtime.
import {writeFileSync} from 'node:fs';
import {METHOD_SPECS} from '../.upstream/src/spec/methods';
import {NAMESPACE_COMMANDS,ALIAS_COMMANDS} from '../.upstream/src/cli/manifest';
writeFileSync(new URL('../internal/operations/methods.json',import.meta.url),JSON.stringify(METHOD_SPECS,null,2)+'\n');
writeFileSync(new URL('../internal/cli/commands.json',import.meta.url),JSON.stringify({namespaces:NAMESPACE_COMMANDS,aliases:ALIAS_COMMANDS},null,2)+'\n');
for(const s of METHOD_SPECS)console.log(`${s.path.join('.')} ${s.argShape} ${s.positional?.map(p=>p.name+(p.required?'!':'')).join(',')??''} ${s.params?.map(p=>p.flag+(p.field?'='+p.field:'')).join(',')??''}`)
console.log(`${METHOD_SPECS.length} methods, ${ALIAS_COMMANDS.length} aliases`)
