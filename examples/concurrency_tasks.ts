const task = spawn((): void => {
  console.log(42);
});

join(task);
yieldNow();
console.log(7);
