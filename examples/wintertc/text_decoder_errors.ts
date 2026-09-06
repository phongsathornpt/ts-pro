const decoder = new TextDecoder();
const malformed = new Uint8Array(2);
malformed[0] = 0xc0;
malformed[1] = 0xaf;
console.log(decoder.decode(malformed));

const fatal = new TextDecoder("utf-8", { fatal: true });
try {
  fatal.decode(malformed);
} catch (error) {
  console.log(error.name);
}

const surrogate = new Uint8Array(3);
surrogate[0] = 0xed;
surrogate[1] = 0xa0;
surrogate[2] = 0x80;
try { fatal.decode(surrogate); } catch (error) { console.log(error.name); }
const tooHigh = new Uint8Array(4);
tooHigh[0] = 0xf4;
tooHigh[1] = 0x90;
tooHigh[2] = 0x80;
tooHigh[3] = 0x80;
try { fatal.decode(tooHigh); } catch (error) { console.log(error.name); }

const truncated = new Uint8Array(2);
truncated[0] = 0xe2;
truncated[1] = 0x82;
try { fatal.decode(truncated); } catch (error) { console.log(error.name); }

const bom = new Uint8Array(4);
bom[0] = 0xef;
bom[1] = 0xbb;
bom[2] = 0xbf;
bom[3] = 65;
console.log(decoder.decode(bom));
const keepBom = new TextDecoder("utf-8", { ignoreBOM: true });
const kept = keepBom.decode(bom);
const keptBytes = new TextEncoder().encode(kept);
console.log(keptBytes.length);
console.log(keptBytes[0]);
console.log(keptBytes[1]);
console.log(keptBytes[2]);
console.log(keptBytes[3]);
