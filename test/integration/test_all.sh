#!/bin/bash

./test/integration/test.sh limited-trust || true
./test/integration/test.sh passthrough || true
