function applyOffset(base: number): number {
  const add = (x: number): number => base + x;
  return add(7);
}

console.log(applyOffset(5));
