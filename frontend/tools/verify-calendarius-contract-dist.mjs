import { readFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';

const sourceManifestPath = 'libs/extensions/calendarius/contract/package.json';
const distRoot = 'dist/libs/extensions/calendarius/contract';
const distManifestPath = join(distRoot, 'package.json');

const sourceManifest = JSON.parse(await readFile(sourceManifestPath, 'utf8'));
const distManifest = JSON.parse(await readFile(distManifestPath, 'utf8'));

if (sourceManifest.version !== distManifest.version) {
  throw new Error(
    `source version ${sourceManifest.version} does not match dist version ${distManifest.version}`,
  );
}

const declarations = [];
async function collectDeclarations(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await collectDeclarations(path);
    } else if (entry.isFile() && entry.name.endsWith('.d.ts')) {
      declarations.push(await readFile(path, 'utf8'));
    }
  }
}

await collectDeclarations(distRoot);
const emittedDeclarations = declarations.join('\n');
for (const requiredSymbol of [
  'EventHappeningDto',
  'CreateEventHappeningRequestDto',
  'UpdateEventHappeningRequestDto',
]) {
  if (!emittedDeclarations.includes(requiredSymbol)) {
    throw new Error(`publishable declarations are missing ${requiredSymbol}`);
  }
}

console.log(
  `verified @sneat/extension-calendarius-contract ${distManifest.version}: ${declarations.length} declaration files contain the EventHappening API`,
);
