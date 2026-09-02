let doCount: number = 0;
let doTotal: number = 0;

do {
  doTotal = doTotal + doCount;
  doCount++;
} while (doCount < 5);

let doOnce: number = 0;
do {
  doOnce++;
} while (false);

console.log(doTotal);
console.log(doOnce);
