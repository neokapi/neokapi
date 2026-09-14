#!/usr/bin/env ruby
# frozen_string_literal: true

# Writes the comment spans the Ruby comment tests compare with.
#
# The spans come from Ripper, the lexer Ruby's own parser is built on, which
# shares no code with the grammar the plugin reads Ruby with. Each fixture
# under corpus/ruby/ has a golden beside it, <fixture>.ripper, holding the Ruby
# version, the file the fixture was copied from, the fixture's sha256, and one
# line per comment: start and end byte offsets, then the lengths of its opening
# and closing markers. Ripper reports a column in bytes, so each is added to its
# line's byte offset. A `=begin` block is one comment from `=begin` through
# `=end`, and a comment ends before its line break.
#
# Usage, from the repository root:
#
#   ruby plugins/sourcecode/internal/comments/testdata/ruby-goldens.rb
#       regenerates every golden
#   ruby plugins/sourcecode/internal/comments/testdata/ruby-goldens.rb --add <file>...
#       copies repository files into the corpus and writes their goldens

require "digest"
require "fileutils"
require "ripper"

HERE = File.expand_path(__dir__)
CORPUS = File.join(HERE, "corpus", "ruby")

def spans(src)
  starts = [0]
  src.each_line { |line| starts << (starts.last + line.bytesize) }
  out = []
  embdoc = nil
  Ripper.lex(src).each do |(line, column), kind, token|
    case kind
    when :on_comment
      out << [starts[line - 1] + column, token.bytesize, 1, 0]
    when :on_embdoc_beg
      embdoc = [starts[line - 1] + column, token.bytesize]
    when :on_embdoc
      embdoc[1] += token.bytesize
    when :on_embdoc_end
      out << [embdoc[0], embdoc[1] + token.bytesize, 6, 4]
      embdoc = nil
    end
  end
  out.map do |start, size, open, close|
    finish = start + size
    finish -= 1 while finish > start && [10, 13].include?(src.getbyte(finish - 1))
    [start, finish, open, close]
  end
end

def source_of(golden)
  return "authored" unless File.exist?(golden)

  line = File.readlines(golden).find { |l| l.start_with?("# source ") }
  line ? line.delete_prefix("# source ").chomp : "authored"
end

def write(fixture, source)
  src = File.binread(fixture)
  lines = [
    "# comment spans from Ripper, Ruby #{RUBY_VERSION}",
    "# source #{source}",
    "# sha256 #{Digest::SHA256.hexdigest(src)}",
  ]
  spans(src).each { |span| lines << span.join(" ") }
  File.write("#{fixture}.ripper", "#{lines.join("\n")}\n")
end

if ARGV.first == "--add"
  root = `git -C #{HERE} rev-parse --show-toplevel`.strip
  FileUtils.mkdir_p(CORPUS)
  ARGV.drop(1).each do |arg|
    path = File.expand_path(arg)
    fixture = File.join(CORPUS, "#{path.split("/").last(2).join("__")}.txt")
    FileUtils.cp(path, fixture)
    write(fixture, path.delete_prefix("#{root}/"))
    puts "added #{File.basename(fixture)}"
  end
else
  Dir.glob(File.join(CORPUS, "*.txt")).sort.each do |fixture|
    write(fixture, source_of("#{fixture}.ripper"))
  end
end
