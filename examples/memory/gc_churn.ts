function churnStrings(n: number): string {
  let value: string = "";
  for (let i = 0; i < n; i++) {
    value = value + "x";
  }
  return "gc-ok";
}

console.log(churnStrings(20000));
