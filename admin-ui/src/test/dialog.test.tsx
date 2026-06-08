import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Dialog } from '../components/Dialog';

describe('Dialog', () => {
  it('renders an accessible modal and closes through the close control', async () => {
    const onClose = vi.fn();
    render(
      <Dialog open title="Create item" onClose={onClose}>
        <button type="button">Inside</button>
      </Dialog>,
    );

    expect(screen.getByRole('dialog', { name: 'Create item' })).toBeInTheDocument();
    const inside = screen.getByRole('button', { name: 'Inside' });

    await userEvent.click(inside);
    expect(onClose).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
