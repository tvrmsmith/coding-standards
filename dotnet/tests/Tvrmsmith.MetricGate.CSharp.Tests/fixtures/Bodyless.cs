using System;

namespace Fixtures;

// A declaration carrying no body carries no lines to score, so it gets no span. An interface
// member, an abstract member and a field-like event are the three ways to write one.
public interface IThing
{
    int Id { get; set; }

    int Compute(int a);
}

public abstract class Thing
{
    public abstract int Compute(int a);

    public event EventHandler? Changed;

    public extern int Extern(int a);
}
