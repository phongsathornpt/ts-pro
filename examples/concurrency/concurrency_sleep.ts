const nativeSleepTask = spawn((): void => {
  sleep(20);
});
join(nativeSleepTask);
console.log(42);
