let value = 1;
const arrow = (): void => {
  value = value + 1;
};
arrow();
console.log(value);

const fn = function (): void {
  value = value + 3;
};
fn();
console.log(value);

const nested = (): void => {
  const inner = (): void => {
    value++;
  };
  inner();
};
nested();
console.log(value);

for (let i = 0; i < 50000; i = i + 1) {
  const dead = "capture-pressure-" + i;
}
setTimeout((): void => {
  value = value + 4;
  console.log(value);
}, 0);
