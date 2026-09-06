const encoder = new TextEncoder();
const target = new Uint8Array(4);
const partial = encoder.encodeInto("hé✓", target);
console.log(partial.read);
console.log(partial.written);
console.log(target[0]);
console.log(target[1]);
console.log(target[2]);
console.log(target[3]);

const emoji = new Uint8Array(4);
const emojiResult = encoder.encodeInto("😀", emoji);
console.log(emojiResult.read);
console.log(emojiResult.written);
console.log(emoji[0]);
console.log(emoji[1]);
console.log(emoji[2]);
console.log(emoji[3]);

const backing = new Uint8Array(6);
const view = backing.subarray(1, 5);
const offsetResult = encoder.encodeInto("AB", view);
console.log(offsetResult.read);
console.log(offsetResult.written);
console.log(backing[0]);
console.log(backing[1]);
console.log(backing[2]);
console.log(backing[3]);
