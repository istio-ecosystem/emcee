#!/bin/bash

./test/integration/test.sh limited-trust auto || true
./test/integration/test.sh passthrough auto || true
