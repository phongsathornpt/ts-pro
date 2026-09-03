export function classify(val: number): string {
  switch (val) {
    case 1:
      return "one";
    case 2:
      return "two";
    case 3:
      return "three";
    default:
      return "other";
  }
}

export function processAction(action: string): number {
  let score = 0;
  switch (action) {
    case "start":
      score = 100;
      break;
    case "pause":
      score = 50;
      break;
    case "stop":
      score = 0;
      break;
    default:
      score = -1;
      break;
  }
  return score;
}

for (let i = 0; i <= 4; i++) {
  console.log(classify(i));
}

console.log(processAction("start"));
console.log(processAction("pause"));
console.log(processAction("stop"));
console.log(processAction("unknown"));
