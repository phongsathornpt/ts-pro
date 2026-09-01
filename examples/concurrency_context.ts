const contextParent = spawn((): void => {
  setTaskContext("trace-42");
  const contextChild = spawn((): void => {
    sleep(1);
    console.log(taskContext());
  });
  join(contextChild);
  console.log(taskContext());
});
join(contextParent);
