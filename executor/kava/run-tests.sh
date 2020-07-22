#! /bin/bash

# Note: this script should be run from the project root directory (otherwise it can't find the docker-compose.yaml or tests)

# exit on errors
set -e

cd integration_test

# start up the chain
docker-compose up -d kavanode

# wait until the node is operational
echo "waiting for kava node to start"
while ! docker-compose exec kavanode curl --fail localhost:26657/status > /dev/null
do
    sleep 1
done
# wait until a block is committed
sleep 4
echo "done"

# run tests
# don't exit on error, just capture exit code (https://stackoverflow.com/questions/11231937/bash-ignoring-error-for-a-particular-command)
# use -count=1 to disable test result caching
go test ../executor/kava -count=1 -tags integration -v && exitStatus=$? || exitStatus=$?

# remove the deputy and chains
docker-compose down

exit $exitStatus
