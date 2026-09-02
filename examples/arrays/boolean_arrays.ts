function boolArrayScore(value: boolean): number {
  if (value) return 1;
  return 0;
}

const flags: boolean[] = [true, false, true];
console.log(flags.length);
console.log(boolArrayScore(flags[0]!));
flags[1] = true;
for (let i = 0; i < 20000; i++) {
  const churn = "g" + "c";
}
console.log(boolArrayScore(flags[1]!));
console.log(boolArrayScore(flags[2]!));
