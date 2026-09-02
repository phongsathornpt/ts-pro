const delayedOuter = spawn((): number => {
  const delayedChild = spawn((): number => {
    sleep(2);
    return 40;
  });
  sleep(1);
  return join(delayedChild) + 2;
});

const delayedVoidOuter = spawn((): number => {
  const delayedVoidChild = spawn((): void => {
    sleep(2);
  });
  sleep(1);
  join(delayedVoidChild);
  return 42;
});

console.log(join(delayedOuter));
console.log(join(delayedVoidOuter));
