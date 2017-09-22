#!/bin/bash

set -e
set -u
set -x

# These variables should not change much
USAGE="Usage: $0 <project>"
PROJECT=${1:?Please provide project name: $USAGE}
SCP_FILES="certs"
IMAGE_TAG="ndt-cloud"
GCE_IMG_PROJECT="cos-cloud"
GCE_IMG_FAMILY="cos-stable"

# Add gcloud to PATH.
source "${HOME}/google-cloud-sdk/path.bash.inc"

# Set the project and zone for all future gcloud commands.
#gcloud config set project $PROJECT
#gcloud config set compute/zone $GCE_ZONE

function create () {
  local NAME=$1
  local ZONE=$2

  # Create a new GCE instance.
  # gcloud compute instances create $NAME --zone $ZONE --project $PROJECT \
  #    --image-project $GCE_IMG_PROJECT \
  #    --image-family $GCE_IMG_FAMILY \
  #    --tags ${IMAGE_TAG} \
  #    --machine-type n1-standard-8

  # Copy required files to the GCE instance.
  # gcloud beta compute scp --recurse --zone $ZONE --project $PROJECT ${SCP_FILES} $NAME:~

  # Build the Docker container.
  # gcloud --zone $ZONE compute ssh $NAME --command "docker build -t ${IMAGE_TAG} ."

  # Start a new container based on the new/updated image
  gcloud compute ssh --zone $ZONE --project $PROJECT $NAME \
      --command 'docker run --net=host -d -v $PWD/certs:/certs -v $PWD/logs:/var/spool/iupui_ndt pboothe/ndt-cloud:6122c37b15d80b6f84aebd47760fa39e4c570862'

  # Needed only if this rule is not propagated successfully.
  gcloud compute ssh --zone $ZONE --project $PROJECT $NAME \
      --command 'sudo iptables -A INPUT -p tcp -j ACCEPT'

}

set -x
set -e
create ndt-iupui-mlab1-tyo01 asia-northeast1-a
create ndt-iupui-mlab1-tyo02 asia-northeast1-b
create ndt-iupui-mlab1-tyo03 asia-northeast1-c
