# Stop when VERSION is `latest`. The docker push targets push $(VERSION), and
# `latest` belongs to the release and its weekly rebuild. Include this at the
# top level of a Makefile that pushes $(VERSION).
# scripts/check-release-build-only.py trusts $(VERSION) only there.
ifeq ($(VERSION),latest)
$(error VERSION=latest: the docker targets push a version tag; latest belongs to the release)
endif
