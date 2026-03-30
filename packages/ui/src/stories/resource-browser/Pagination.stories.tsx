import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Pagination } from "../../components/resource-browser/Pagination";

const meta: Meta<typeof Pagination> = {
  title: "Resource Browser/Pagination",
  component: Pagination,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 500, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Pagination controls with 'Showing X to Y of Z' summary and Previous/Next buttons. Hidden when total pages is 1 or fewer.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof Pagination>;

export const FirstPage: Story = {
  args: {
    page: 0,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {},
  },
};

export const MiddlePage: Story = {
  args: {
    page: 3,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {},
  },
};

export const LastPage: Story = {
  args: {
    page: 5,
    pageSize: 50,
    totalCount: 284,
    onPageChange: () => {},
  },
};

/** Interactive pagination that tracks the current page. */
export const Interactive: Story = {
  render: function InteractivePagination() {
    const [page, setPage] = useState(0);
    return <Pagination page={page} pageSize={50} totalCount={284} onPageChange={setPage} />;
  },
};
