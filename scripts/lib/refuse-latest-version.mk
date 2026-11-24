# Stop unless VERSION is a Docker tag other than `latest`. The docker push
# targets push $(VERSION), and `latest` belongs to the release and its weekly
# rebuild. Include this at the top level of a Makefile that pushes
# $(VERSION). scripts/check-release-build-only.py trusts $(VERSION) only there.
#
# The rule matches .github/actions/refuse-latest-tag: the tag grammar
# [A-Za-z0-9_][A-Za-z0-9_.-]{0,127}, and not `latest`. A value with
# whitespace fails, since the shell splits it and `latest ` would push latest.
_rlv_allowed := a b c d e f g h i j k l m n o p q r s t u v w x y z \
  A B C D E F G H I J K L M N O P Q R S T U V W X Y Z 0 1 2 3 4 5 6 7 8 9 _ . -
# What is left of $(1) once every allowed character is removed.
_rlv_rest = $(if $(2),$(call _rlv_rest,$(subst $(firstword $(2)),,$(1)),$(wordlist 2,999,$(2))),$(1))
# $(1) as one word per character, each written `|`, to count its length.
_rlv_spell = $(if $(2),$(call _rlv_spell,$(subst $(firstword $(2)),| ,$(1)),$(wordlist 2,999,$(2))),$(1))
_rlv_v := $(strip $(VERSION))
_rlv_bad :=
ifneq ($(words $(VERSION)),1)
  _rlv_bad := it is empty or holds whitespace
else ifneq ($(VERSION),$(_rlv_v))
  _rlv_bad := it holds whitespace
else ifneq ($(call _rlv_rest,$(_rlv_v),$(_rlv_allowed)),)
  _rlv_bad := it holds a character outside A-Z a-z 0-9 _ . -
else ifneq ($(filter .% -%,$(_rlv_v)),)
  _rlv_bad := it starts with . or -
else ifneq ($(word 129,$(call _rlv_spell,$(_rlv_v),$(_rlv_allowed))),)
  _rlv_bad := it is longer than 128 characters
else ifeq ($(_rlv_v),latest)
  _rlv_bad := latest belongs to the release
endif
ifneq ($(_rlv_bad),)
$(error VERSION='$(VERSION)' is not a tag the docker targets may push: $(_rlv_bad))
endif
