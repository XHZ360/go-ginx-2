import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Dialog } from '../components/Dialog';

describe('Dialog', () => {
  it('closes only when a complete click starts and ends on the backdrop', async () => {
    const onClose = vi.fn();
    render(
      <Dialog open title="Create item" onClose={onClose}>
        <button type="button">Inside</button>
      </Dialog>,
    );

    const backdrop = screen.getByRole('presentation');
    const inside = screen.getByRole('button', { name: 'Inside' });

    await userEvent.pointer([
      { keys: '[MouseLeft>]', target: inside },
      { keys: '[/MouseLeft]', target: backdrop },
    ]);
    expect(onClose).not.toHaveBeenCalled();

    await userEvent.pointer([
      { keys: '[MouseLeft>]', target: backdrop },
      { keys: '[/MouseLeft]', target: backdrop },
    ]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
