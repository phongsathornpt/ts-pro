console.log(globalThis === self);
const g: any = globalThis;
g.marker = "alive";
for (let i = 0; i < 50000; i = i + 1) {
  const dead = "value-" + i;
}
const s: any = self;
console.log(s.marker);
