function crossBoolToNumber(value: boolean): number {
  if (value) return 1;
  return 0;
}

const boolReady = channel<number>(1);
const boolInbound = channel<boolean>(0);
const boolReceiver = spawn((): boolean => {
  channelSend(boolReady, 1);
  return channelRecv(boolInbound);
});
channelRecv(boolReady);
sleep(2);
channelSend(boolInbound, true);
console.log(crossBoolToNumber(join(boolReceiver)));

const boolOutbound = channel<boolean>(0);
const boolSender = spawn((): void => {
  channelSend(boolOutbound, false);
});
sleep(2);
console.log(crossBoolToNumber(channelRecv(boolOutbound)));
join(boolSender);
