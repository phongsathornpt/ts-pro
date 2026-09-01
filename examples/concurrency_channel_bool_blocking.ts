function blockingBoolToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const blockingBoolChannel = channel<boolean>(0);
const blockingBoolSender = spawn((): void => {
  channelSend(blockingBoolChannel, true);
});
const blockingBoolReceiver = spawn((): boolean => channelRecv(blockingBoolChannel));
console.log(blockingBoolToNumber(join(blockingBoolReceiver)));
join(blockingBoolSender);
