const multiSuspendChannel = channel<number>(1);
const multiSuspendSender = spawn((): void => {
  sleep(1);
  channelSend(multiSuspendChannel, 42);
  sleep(1);
});
const multiSuspendReceiver = spawn((): number => {
  const value = channelRecv(multiSuspendChannel);
  sleep(1);
  return value;
});
console.log(join(multiSuspendReceiver));
join(multiSuspendSender);
