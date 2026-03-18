import type { Meta, StoryObj } from "@storybook/react-vite";
import { BravoCodeBlock } from "../../components/bravo/BravoCodeBlock";

const meta: Meta<typeof BravoCodeBlock> = {
  title: "Bravo/BravoCodeBlock",
  component: BravoCodeBlock,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ maxWidth: 480, padding: 16 }}><Story /></div>],
};

export default meta;
type Story = StoryObj<typeof BravoCodeBlock>;

export const PythonCode: Story = {
  args: {
    language: "python",
    code: `import json

with open("en-US.json") as f:
    data = json.load(f)

print(f"Keys: {len(data)}")
for key in list(data.keys())[:5]:
    print(f"  {key}: {data[key]}")`,
  },
};

export const WithResult: Story = {
  args: {
    language: "python",
    code: `print("Hello from sandbox!")`,
    result: {
      stdout: "Hello from sandbox!\n",
      stderr: "",
      exit_code: 0,
    },
  },
};

export const WithError: Story = {
  args: {
    language: "bash",
    code: `cat /etc/nonexistent`,
    result: {
      stdout: "",
      stderr: "cat: /etc/nonexistent: No such file or directory\n",
      exit_code: 1,
    },
  },
};

export const NodeScript: Story = {
  args: {
    language: "node",
    code: `const fs = require('fs');
const data = JSON.parse(fs.readFileSync('/workspace/messages.json', 'utf8'));
console.log(\`Found \${Object.keys(data).length} messages\`);`,
  },
};
