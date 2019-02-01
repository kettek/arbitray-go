#/bin/sh

#if [ -z "$GOPATH" ]; then
#  echo GOPATH environment variable not set
#  exit
#fi

if ! [ -x "$(command -v 2goarray)" ]; then
  echo "Installing 2goarray..."
  go get github.com/cratonica/2goarray
  if [ $? -ne 0 ]; then
    echo Failure executing go get github.com/cratonica/2goarray
    exit
  fi
fi

OUTPUT=go/ArbitrayIcon.go
echo Generating $OUTPUT
echo "//+build linux darwin" > $OUTPUT
echo >> $OUTPUT
cat "resources/arbitray.png" | 2goarray iconData main >> $OUTPUT
if [ $? -ne 0 ]; then
  echo Failure generating $OUTPUT
  exit
fi
echo Finished