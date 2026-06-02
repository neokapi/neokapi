package xcstrings_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xcstrings"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skeletonRoundtripXC reads input through the xcstrings reader with a skeleton
// store wired, then writes it back through the writer fed the same store, with
// no translation applied. The skeleton path is what `kapi merge` uses (the
// returning file's blocks are spliced into the source-captured skeleton), so a
// byte-exact identity roundtrip here is the merge byte-exactness guarantee.
func skeletonRoundtripXC(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := xcstrings.NewReader()
	writer := xcstrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	require.Positive(t, store.EntriesWritten(), "reader must emit skeleton entries")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String()
}

// TestSkeletonStore_ByteExact_XC pins the skeleton emit/consume path the
// xcstrings reader/writer perform — every non-translatable byte is replayed
// verbatim from the skeleton, and each translatable leaf value is re-encoded
// from its block. An untranslated roundtrip must be byte-for-byte identical.
// Real-fixture coverage of the original-bytes (non-skeleton) path lives in
// TestByteFaithfulRoundTrip / TestCorpusByteFaithfulRoundTrip; these snippets
// pin the merge (skeleton) path.
func TestSkeletonStore_ByteExact_XC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "simple_stringunit",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Cancel" : {
      "comment" : "Button title",
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Abbrechen"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "empty_localizations_entry",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Settings" : {

    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "printf_placeholder_value",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Hello, %@!" : {
      "extractionState" : "manual",
      "localizations" : {
        "fr" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Bonjour, %@ !"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "plural_variations",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "%lld items selected" : {
      "extractionState" : "manual",
      "localizations" : {
        "en" : {
          "variations" : {
            "plural" : {
              "one" : {
                "stringUnit" : {
                  "state" : "translated",
                  "value" : "%lld item selected"
                }
              },
              "other" : {
                "stringUnit" : {
                  "state" : "translated",
                  "value" : "%lld items selected"
                }
              }
            }
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "device_variations",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Tap to continue" : {
      "extractionState" : "manual",
      "localizations" : {
        "en" : {
          "variations" : {
            "device" : {
              "iphone" : {
                "stringUnit" : {
                  "state" : "translated",
                  "value" : "Tap to continue"
                }
              },
              "mac" : {
                "stringUnit" : {
                  "state" : "translated",
                  "value" : "Click to continue"
                }
              }
            }
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "substitutions",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "%1$@ has %2$lld photos in %3$@" : {
      "extractionState" : "manual",
      "localizations" : {
        "en" : {
          "variations" : {
            "substitutions" : {
              "photo_count" : {
                "argNum" : 2,
                "formatSpecifier" : "lld",
                "variations" : {
                  "plural" : {
                    "one" : {
                      "stringUnit" : {
                        "state" : "translated",
                        "value" : "%1$@ has %arg photo in %3$@"
                      }
                    },
                    "other" : {
                      "stringUnit" : {
                        "state" : "translated",
                        "value" : "%1$@ has %arg photos in %3$@"
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			name: "multiple_entries_and_langs",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "Cancel" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Abbrechen"
          }
        },
        "fr" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Annuler"
          }
        }
      }
    },
    "OK" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "OK"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			// Escapes that encodeJSONString reproduces canonically (\n, \t,
			// \", \\) round-trip byte-exact through the re-encode path.
			name: "canonical_escapes",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "multiline" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Zeile eins\nZeile zwei\t\"zitiert\"\\"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			// Literal non-ASCII UTF-8 (Apple's encoder does not escape it),
			// which encodeJSONString emits verbatim.
			name: "unicode_literal",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "greeting" : {
      "localizations" : {
        "ru" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Выбрано %lld элемента"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			// Compact (no pretty-printing) — the skeleton preserves the absence
			// of whitespace just as faithfully as it preserves it.
			name:  "compact",
			input: `{"sourceLanguage":"en","strings":{"Cancel":{"localizations":{"de":{"stringUnit":{"state":"translated","value":"Abbrechen"}}}}},"version":"1.0"}`,
		},
		{
			// A stale entry: with ExtractStale defaulting true the reader emits
			// a block for it, so the skeleton carries a Ref and the value
			// re-encodes byte-exact. (When ExtractStale is off the leaf is
			// skipped on both the block and skeleton sides — see the package
			// note on stale alignment.)
			name: "stale_entry_default_extracts",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "OldKey" : {
      "extractionState" : "stale",
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Veraltet"
          }
        }
      }
    },
    "NewKey" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Neu"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.input, skeletonRoundtripXC(t, tc.input),
				"skeleton store roundtrip should be byte-exact")
		})
	}
}

// TestSkeletonStore_WithTranslation_XC exercises the re-encode path the
// byte-exact (untranslated) test skips: every other byte is replayed from the
// skeleton verbatim and only the one translated leaf value changes.
func TestSkeletonStore_WithTranslation_XC(t *testing.T) {
	t.Parallel()
	input := `{
  "sourceLanguage" : "en",
  "strings" : {
    "Cancel" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Abbrechen"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`
	want := `{
  "sourceLanguage" : "en",
  "strings" : {
    "Cancel" : {
      "localizations" : {
        "de" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "Stornieren"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := xcstrings.NewReader()
	writer := xcstrings.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Retarget only the German leaf value; every other byte must be replayed
	// from the skeleton unchanged.
	changed := 0
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Properties["xcstrings.lang"] == "de" {
			b.SetTargetText(locale, "Stornieren")
			changed++
		}
	}
	require.Equal(t, 1, changed, "exactly one German leaf should be present")

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()

	assert.Equal(t, want, buf.String(),
		"only the chosen leaf value should change; all other bytes replayed verbatim")
}
