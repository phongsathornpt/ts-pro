const refReady = channel<number>(1);
const refInbound = channel<string>(0);
const refReceiver = spawn((): string => {
  channelSend(refReady, 1);
  return channelRecv(refInbound);
});
channelRecv(refReady);
sleep(2);
channelSend(refInbound, "external-to-task");
console.log(join(refReceiver));

const refOutbound = channel<string>(0);
const refSender = spawn((): void => {
  channelSend(refOutbound, "task-to-external");
});
sleep(2);
console.log(channelRecv(refOutbound));
join(refSender);
